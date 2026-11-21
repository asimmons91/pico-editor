// Package workspace scans a project folder, builds the file tree model, and
// associates included .lua files with their owning carts through pkg/cart.
package workspace

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	p8cart "github.com/asimmons91/pico-lsp/pkg/cart"
)

var errSkipDir = errors.New("skip directory")

// MaxEntries is the default V1 scan cap. Hitting it stops the scan with
// Truncated set. Files settings may override it.
const MaxEntries = 20000

// DefaultExclude is the documented file tree exclusion list.
var DefaultExclude = []string{".git", "node_modules", "vendor", "bin", "dist"}

// Options control the workspace scan. The zero value is not useful; call
// DefaultOptions and override fields.
type Options struct {
	ShowHidden bool
	Exclude    []string
	MaxEntries int
}

// DefaultOptions returns the M7 defaults.
func DefaultOptions() Options {
	return Options{
		ShowHidden: false,
		Exclude:    append([]string(nil), DefaultExclude...),
		MaxEntries: MaxEntries,
	}
}

func (o Options) normalized() Options {
	if o.MaxEntries <= 0 {
		o.MaxEntries = MaxEntries
	}
	if o.Exclude == nil {
		o.Exclude = append([]string(nil), DefaultExclude...)
	}
	return o
}

// maxCartScanSize mirrors the cart adapter's open limit so workspace scans do
// not read arbitrarily large files just to discover they are unsupported.
const maxCartScanSize = 2 * 1024 * 1024

// Entry is one file or directory in the workspace tree. Children are sorted
// directories first, then case-insensitive name.
type Entry struct {
	Path     string
	Name     string
	Dir      bool
	Children []*Entry
}

// CartRef is the scanned metadata for one .p8 cart.
type CartRef struct {
	Path     string
	Tabs     []p8cart.Tab
	Includes []p8cart.Include
	Document *p8cart.Document
}

// Workspace is one project root plus its file tree and include associations.
type Workspace struct {
	Root      string
	Entries   []*Entry
	Carts     map[string]*CartRef
	Owners    map[string]string // .lua path -> owning .p8 path
	Warnings  []string
	Truncated bool

	logger   *slog.Logger
	resolver *p8cart.Resolver
	count    int
	options  Options
	exclude  map[string]bool
}

// Open scans root with the default options. It returns an error only when root
// itself is unusable; individual unreadable children are reported as warnings.
func Open(root string) (*Workspace, error) {
	return OpenWithLogger(root, nil)
}

// OpenWithLogger is Open with a logger for skipped directories and carts.
func OpenWithLogger(root string, logger *slog.Logger) (*Workspace, error) {
	return OpenWithOptions(root, DefaultOptions(), logger)
}

// OpenWithOptions scans root using explicit M7 file settings.
func OpenWithOptions(root string, options Options, logger *slog.Logger) (*Workspace, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace root %s: %w", root, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("workspace root %s: %w", absolute, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root %s: not a directory", absolute)
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	options = options.normalized()
	workspace := &Workspace{
		Root:     filepath.Clean(absolute),
		Carts:    make(map[string]*CartRef),
		Owners:   make(map[string]string),
		logger:   logger,
		resolver: p8cart.NewResolver(nil),
		options:  options,
		exclude:  excludeSet(options.Exclude),
	}
	if err := workspace.Refresh(); err != nil {
		return nil, err
	}
	return workspace, nil
}

// SetOptions replaces the scan settings for the next refresh.
func (w *Workspace) SetOptions(options Options) {
	w.options = options.normalized()
	w.exclude = excludeSet(w.options.Exclude)
}

// Options returns the active scan settings.
func (w *Workspace) Options() Options {
	return w.options
}

func excludeSet(entries []string) map[string]bool {
	set := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry != "" {
			set[entry] = true
		}
	}
	return set
}

// Refresh rescans the root. The previous tree is replaced only on success.
func (w *Workspace) Refresh() error {
	w.count = 0
	w.Truncated = false
	w.Warnings = nil
	entries, err := w.scanDir(w.Root)
	if err != nil {
		return err
	}
	w.Entries = entries
	w.Carts = make(map[string]*CartRef)
	w.Owners = make(map[string]string)
	w.resolver.Invalidate()
	w.scanCarts()
	w.scanOwners()
	return nil
}

// ResolveInclude returns the nearest workspace cart that includes path.
func (w *Workspace) ResolveInclude(path string) (*CartRef, bool) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, false
	}
	absolute = filepath.Clean(absolute)
	if strings.EqualFold(filepath.Ext(absolute), ".lua") {
		if ref := w.Carts[absolute]; ref != nil {
			return ref, true
		}
	}
	if owner, ok := w.Owners[absolute]; ok {
		if ref := w.Carts[owner]; ref != nil {
			return ref, true
		}
	}
	owner, err := w.resolver.Owner(absolute)
	if err != nil || owner == absolute || !w.withinRoot(owner) {
		return nil, false
	}
	if ref, ok := w.Carts[owner]; ok {
		return ref, true
	}
	ref := w.loadCart(owner)
	if ref == nil {
		return nil, false
	}
	w.Carts[owner] = ref
	w.Owners[absolute] = owner
	return ref, true
}

func (w *Workspace) scanDir(dir string) ([]*Entry, error) {
	if w.Truncated {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if dir == w.Root {
			return nil, err
		}
		w.warn("skip directory", dir, err)
		return nil, errSkipDir
	}
	children := make([]*Entry, 0, len(entries))
	for _, entry := range entries {
		if w.Truncated {
			break
		}
		name := entry.Name()
		if w.ignoreName(name) {
			continue
		}
		path := filepath.Join(dir, name)
		mode := entry.Type()
		if mode&os.ModeSymlink != 0 {
			info, statErr := os.Stat(path)
			if statErr != nil {
				w.warn("skip symlink", path, statErr)
				continue
			}
			if info.IsDir() {
				// Directory symlinks are not followed, which prevents loops.
				continue
			}
			if !info.Mode().IsRegular() || !wantedFile(name) {
				continue
			}
			child, ok := w.addFile(path, name)
			if ok {
				children = append(children, child)
			}
			continue
		}
		if entry.IsDir() {
			child, ok := w.addDir(path, name)
			if ok {
				children = append(children, child)
			}
			continue
		}
		if !entry.Type().IsRegular() || !wantedFile(name) {
			continue
		}
		child, ok := w.addFile(path, name)
		if ok {
			children = append(children, child)
		}
	}
	sortEntries(children)
	return children, nil
}

func (w *Workspace) addDir(path, name string) (*Entry, bool) {
	if !w.reserve() {
		return nil, false
	}
	children, err := w.scanDir(path)
	if err != nil {
		if errors.Is(err, errSkipDir) {
			return nil, false
		}
		w.warn("scan directory", path, err)
		return nil, false
	}
	entry := &Entry{Path: path, Name: name, Dir: true, Children: children}
	return entry, true
}

func (w *Workspace) addFile(path, name string) (*Entry, bool) {
	if !w.reserve() {
		return nil, false
	}
	return &Entry{Path: path, Name: name}, true
}

func (w *Workspace) reserve() bool {
	if w.count >= w.options.MaxEntries {
		if !w.Truncated {
			w.Truncated = true
			w.warn("file tree truncated", w.Root, fmt.Errorf("more than %d entries", w.options.MaxEntries))
		}
		return false
	}
	w.count++
	return true
}

func (w *Workspace) scanCarts() {
	var carts []string
	w.walk(func(entry *Entry) {
		if !entry.Dir && strings.EqualFold(filepath.Ext(entry.Name), ".p8") {
			carts = append(carts, entry.Path)
		}
	})
	sort.Strings(carts)
	for _, path := range carts {
		ref := w.loadCart(path)
		if ref != nil {
			w.Carts[path] = ref
		}
	}
}

func (w *Workspace) loadCart(path string) *CartRef {
	info, err := os.Stat(path)
	if err != nil {
		w.warn("skip cart", path, err)
		return nil
	}
	if !info.Mode().IsRegular() {
		w.warn("skip cart", path, errors.New("not a regular file"))
		return nil
	}
	if info.Size() > maxCartScanSize {
		w.warn("skip cart", path, fmt.Errorf("cart is larger than %d bytes", maxCartScanSize))
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		w.warn("skip cart", path, err)
		return nil
	}
	if len(data) > maxCartScanSize {
		w.warn("skip cart", path, fmt.Errorf("cart is larger than %d bytes", maxCartScanSize))
		return nil
	}
	doc, err := p8cart.Parse(path, data)
	if err != nil {
		w.warn("skip cart", path, err)
		return nil
	}
	ref := &CartRef{
		Path:     path,
		Tabs:     doc.Tabs(),
		Includes: doc.Includes(),
		Document: doc,
	}
	for _, include := range ref.Includes {
		if include.ResolvedPath == "" {
			continue
		}
		if existing, ok := w.Carts[include.ResolvedPath]; !ok || betterOwner(path, existing.Path, include.ResolvedPath) {
			w.Carts[include.ResolvedPath] = ref
		}
	}
	return ref
}

func (w *Workspace) scanOwners() {
	var files []string
	w.walk(func(entry *Entry) {
		if !entry.Dir && strings.EqualFold(filepath.Ext(entry.Name), ".lua") {
			files = append(files, entry.Path)
		}
	})
	sort.Strings(files)
	for _, path := range files {
		if ref := w.Carts[path]; ref != nil {
			w.Owners[path] = ref.Path
			continue
		}
		owner, err := w.resolver.Owner(path)
		if err != nil || owner == path || !w.withinRoot(owner) {
			continue
		}
		w.Owners[path] = owner
		if _, ok := w.Carts[owner]; !ok {
			if ref := w.loadCart(owner); ref != nil {
				w.Carts[owner] = ref
			}
		}
	}
}

func (w *Workspace) walk(visit func(*Entry)) {
	var walk func([]*Entry)
	walk = func(entries []*Entry) {
		for _, entry := range entries {
			visit(entry)
			if entry.Dir {
				walk(entry.Children)
			}
		}
	}
	walk(w.Entries)
}

func (w *Workspace) withinRoot(path string) bool {
	relative, err := filepath.Rel(w.Root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (w *Workspace) warn(message, path string, err error) {
	text := fmt.Sprintf("%s %s: %v", message, path, err)
	w.Warnings = append(w.Warnings, text)
	w.logger.Warn(message, "path", path, "error", err)
}

func sortEntries(entries []*Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if left.Dir != right.Dir {
			return left.Dir
		}
		lname := strings.ToLower(left.Name)
		rname := strings.ToLower(right.Name)
		if lname != rname {
			return lname < rname
		}
		return left.Name < right.Name
	})
}

func wantedFile(name string) bool {
	extension := filepath.Ext(name)
	return strings.EqualFold(extension, ".p8") || strings.EqualFold(extension, ".lua")
}

func (w *Workspace) ignoreName(name string) bool {
	if w.exclude[name] {
		return true
	}
	if !w.options.ShowHidden && strings.HasPrefix(name, ".") {
		return true
	}
	return false
}

func betterOwner(candidate, current, target string) bool {
	candidateDistance := directoryDistance(candidate, target)
	currentDistance := directoryDistance(current, target)
	if candidateDistance != currentDistance {
		return candidateDistance < currentDistance
	}
	return candidate < current
}

func directoryDistance(cartPath, target string) int {
	relative, err := filepath.Rel(filepath.Dir(cartPath), filepath.Dir(target))
	if err != nil {
		return 1 << 30
	}
	if relative == "." {
		return 0
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	distance := 0
	for _, part := range parts {
		switch part {
		case "", ".":
		case "..":
			distance++
		default:
			distance++
		}
	}
	return distance
}
