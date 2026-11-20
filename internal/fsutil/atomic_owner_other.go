//go:build !unix

package fsutil

import "os"

func preserveOwner(string, os.FileInfo) error {
	return nil
}
