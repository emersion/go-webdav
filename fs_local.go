package webdav

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/emersion/go-webdav/internal"
)

// LocalFileSystem implements FileSystem for a local directory.
type LocalFileSystem string

var _ FileSystem = LocalFileSystem("")

func (fs LocalFileSystem) localPath(name string) (string, error) {
	if (filepath.Separator != '/' && strings.IndexRune(name, filepath.Separator) >= 0) || strings.Contains(name, "\x00") {
		return "", internal.HTTPErrorf(http.StatusBadRequest, "webdav: invalid character in path")
	}
	name = path.Clean(name)
	if !path.IsAbs(name) {
		return "", internal.HTTPErrorf(http.StatusBadRequest, "webdav: expected absolute path, got %q", name)
	}
	return filepath.Join(string(fs), filepath.FromSlash(name)), nil
}

func (fs LocalFileSystem) externalPath(name string) (string, error) {
	rel, err := filepath.Rel(string(fs), name)
	if err != nil {
		return "", err
	}
	return "/" + filepath.ToSlash(rel), nil
}

func (fs LocalFileSystem) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	p, err := fs.localPath(name)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func fileInfoFromOS(p string, fi os.FileInfo) *FileInfo {
	return &FileInfo{
		Path:    p,
		Size:    fi.Size(),
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
		// TODO: fallback to http.DetectContentType?
		MIMEType: mime.TypeByExtension(path.Ext(p)),
		// RFC 2616 section 13.3.3 describes strong ETags. Ideally these would
		// be checksums or sequence numbers, however these are expensive to
		// compute. The modification time with nanosecond granularity is good
		// enough, as it's very unlikely for the same file to be modified twice
		// during a single nanosecond.
		ETag: fmt.Sprintf("%x%x", fi.ModTime().UnixNano(), fi.Size()),
	}
}

func errFromOS(err error) error {
	// Remove path from path errors so it's not returned to the user
	var perr *fs.PathError
	if errors.As(err, &perr) {
		err = fmt.Errorf("%s: %w", perr.Op, perr.Err)
	}

	if errors.Is(err, fs.ErrNotExist) {
		return NewHTTPError(http.StatusNotFound, err)
	} else if errors.Is(err, fs.ErrPermission) {
		return NewHTTPError(http.StatusForbidden, err)
	} else if errors.Is(err, os.ErrDeadlineExceeded) {
		return NewHTTPError(http.StatusServiceUnavailable, err)
	} else {
		return err
	}
}

func (fs LocalFileSystem) Stat(ctx context.Context, name string) (*FileInfo, error) {
	p, err := fs.localPath(name)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		return nil, errFromOS(err)
	}
	return fileInfoFromOS(name, fi), nil
}

func (fs LocalFileSystem) ReadDir(ctx context.Context, name string, recursive bool) ([]FileInfo, error) {
	path, err := fs.localPath(name)
	if err != nil {
		return nil, err
	}

	var l []FileInfo
	err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		href, err := fs.externalPath(p)
		if err != nil {
			return err
		}

		l = append(l, *fileInfoFromOS(href, fi))

		if !recursive && fi.IsDir() && path != p {
			return filepath.SkipDir
		}
		return nil
	})
	return l, errFromOS(err)
}

func checkConditionalMatches(fi *FileInfo, ifMatch, ifNoneMatch ConditionalMatch) error {
	etag := ""
	if fi != nil {
		etag = fi.ETag
	}

	if ifMatch.IsSet() {
		if ok, err := ifMatch.MatchETag(etag); err != nil {
			return NewHTTPError(http.StatusBadRequest, err)
		} else if !ok {
			return NewHTTPError(http.StatusPreconditionFailed, fmt.Errorf("If-Match condition failed"))
		}
	}

	if ifNoneMatch.IsSet() {
		if ok, err := ifNoneMatch.MatchETag(etag); err != nil {
			return NewHTTPError(http.StatusBadRequest, err)
		} else if ok {
			return NewHTTPError(http.StatusPreconditionFailed, fmt.Errorf("If-None-Match condition failed"))
		}
	}

	return nil
}

func (fs LocalFileSystem) Create(ctx context.Context, name string, body io.ReadCloser, opts *CreateOptions) (fi *FileInfo, created bool, err error) {
	p, err := fs.localPath(name)
	if err != nil {
		return nil, false, err
	}
	fi, _ = fs.Stat(ctx, name)
	created = fi == nil

	if err := checkConditionalMatches(fi, opts.IfMatch, opts.IfNoneMatch); err != nil {
		return nil, false, err
	}

	wc, err := os.Create(p)
	if err != nil {
		return nil, false, errFromOS(err)
	}
	defer wc.Close()

	if _, err := io.Copy(wc, body); err != nil {
		os.Remove(p)
		return nil, false, err
	}
	if err := wc.Close(); err != nil {
		os.Remove(p)
		return nil, false, err
	}

	fi, err = fs.Stat(ctx, name)
	if err != nil {
		return nil, false, err
	}

	return fi, created, err
}

func (fs LocalFileSystem) RemoveAll(ctx context.Context, name string, opts *RemoveAllOptions) error {
	p, err := fs.localPath(name)
	if err != nil {
		return err
	}

	// WebDAV semantics are that it should return a "404 Not Found" error in
	// case the resource doesn't exist. We need to Stat before RemoveAll.
	fi, err := fs.Stat(ctx, name)
	if err != nil {
		return errFromOS(err)
	}

	if err := checkConditionalMatches(fi, opts.IfMatch, opts.IfNoneMatch); err != nil {
		return err
	}

	return errFromOS(os.RemoveAll(p))
}

func (fs LocalFileSystem) Mkdir(ctx context.Context, name string) error {
	p, err := fs.localPath(name)
	if err != nil {
		return err
	}
	if err := os.Mkdir(p, 0755); os.IsExist(err) {
		return NewHTTPError(http.StatusMethodNotAllowed, err)
	} else {
		return errFromOS(err)
	}
}

func copyRegularFile(src, dst string, perm os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return errFromOS(err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
	if os.IsNotExist(err) {
		return NewHTTPError(http.StatusConflict, err)
	} else if err != nil {
		return errFromOS(err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Close()
}

func (fs LocalFileSystem) Copy(ctx context.Context, src, dst string, options *CopyOptions) (created bool, err error) {
	srcPath, err := fs.localPath(src)
	if err != nil {
		return false, err
	}
	dstPath, err := fs.localPath(dst)
	if err != nil {
		return false, err
	}

	// TODO: "Note that an infinite-depth COPY of /A/ into /A/B/ could lead to
	// infinite recursion if not handled correctly"

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return false, errFromOS(err)
	}
	srcPerm := srcInfo.Mode() & os.ModePerm

	if _, err := os.Stat(dstPath); err != nil {
		if !os.IsNotExist(err) {
			return false, errFromOS(err)
		}
		created = true
	} else {
		if options.NoOverwrite {
			return false, NewHTTPError(http.StatusPreconditionFailed, os.ErrExist)
		}
		if err := os.RemoveAll(dstPath); err != nil {
			return false, errFromOS(err)
		}
	}

	err = filepath.Walk(srcPath, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() {
			if err := os.Mkdir(dstPath, srcPerm); err != nil {
				return errFromOS(err)
			}
		} else {
			if err := copyRegularFile(srcPath, dstPath, srcPerm); err != nil {
				return err
			}
		}

		if fi.IsDir() && options.NoRecursive {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return false, errFromOS(err)
	}

	return created, nil
}

func (fs LocalFileSystem) Move(ctx context.Context, src, dst string, options *MoveOptions) (created bool, err error) {
	srcPath, err := fs.localPath(src)
	if err != nil {
		return false, err
	}
	dstPath, err := fs.localPath(dst)
	if err != nil {
		return false, err
	}

	if _, err := os.Stat(dstPath); err != nil {
		if !os.IsNotExist(err) {
			return false, errFromOS(err)
		}
		created = true
	} else {
		if options.NoOverwrite {
			return false, NewHTTPError(http.StatusPreconditionFailed, os.ErrExist)
		}
		if err := os.RemoveAll(dstPath); err != nil {
			return false, errFromOS(err)
		}
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		return false, errFromOS(err)
	}

	return created, nil
}
