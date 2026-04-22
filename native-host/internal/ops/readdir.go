package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/safety"
)

// PROTOCOL.md §7.3 defines the args object as:
//
//	{ path, page?, pageSize?, cursor?, sort: { field, order }, includeHidden? }
//
// Sort field ∈ "name" | "size" | "modified" | "type"
// Sort order ∈ "asc" | "desc"
//
// We implement page/pageSize (offset-style paging with numeric pages) for
// Phase 1; opaque cursors are a Phase 2+ nicety. The maximum pageSize is
// capped at 5000 to keep a single response frame comfortably under the 1MB
// wire limit.

const (
	readdirDefaultPageSize = 1000
	readdirMaxPageSize     = 5000
)

type readdirSort struct {
	Field string `json:"field,omitempty"`
	Order string `json:"order,omitempty"`
}

type readdirArgs struct {
	Path          string      `json:"path"`
	Page          int         `json:"page,omitempty"`
	PageSize      int         `json:"pageSize,omitempty"`
	Cursor        string      `json:"cursor,omitempty"`
	Sort          readdirSort `json:"sort"`
	IncludeHidden bool        `json:"includeHidden,omitempty"`
}

// readdirEntry mirrors PROTOCOL.md §7.3's entry shape. SizeBytes is a *int64
// so directories can emit JSON `null` rather than `0`, which would be
// indistinguishable from a zero-byte file.
type readdirEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"` // "file" | "directory" | "symlink"
	SizeBytes  *int64 `json:"sizeBytes"`
	ModifiedTs int64  `json:"modifiedTs"` // unix millis
	ReadOnly   bool   `json:"readOnly"`
	Hidden     bool   `json:"hidden"`
	Symlink    bool   `json:"symlink,omitempty"`
}

type readdirData struct {
	Entries    []readdirEntry `json:"entries"`
	NextCursor *string        `json:"nextCursor"`
	Total      int            `json:"total"`
}

// Readdir lists the immediate children of args.path with sorting, paging,
// and hidden-file filtering. It does not recurse (Phase 3 `search` covers
// that territory).
func Readdir(ctx context.Context, req protocol.Request) protocol.Response {
	var args readdirArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			return errResp(req.ID, protocol.ErrCodeBadRequest,
				"invalid args: "+err.Error(), false)
		}
	}

	cleaned, err := safety.CleanPath(args.Path)
	if err != nil {
		return errResp(req.ID, protocol.ErrCodePathRejected, err.Error(), false)
	}

	// Cursor takes precedence over Page when both are present. We emit
	// cursors as stringified page numbers (see below) so this decoder is
	// the symmetric inverse of what the response assembles.
	if args.Cursor != "" {
		if n, perr := strconv.Atoi(args.Cursor); perr == nil && n >= 0 {
			args.Page = n
		}
	}

	// pageSize clamp happens before the os.ReadDir so that absurd requests
	// (pageSize=1_000_000) are rejected cheaply rather than after we've
	// loaded a huge directory into memory.
	pageSize := args.PageSize
	if pageSize == 0 {
		pageSize = readdirDefaultPageSize
	}
	if pageSize < 0 {
		return errResp(req.ID, protocol.ErrCodeEINVAL,
			"pageSize must be positive", false)
	}
	if pageSize > readdirMaxPageSize {
		return errResp(req.ID, protocol.ErrCodeTooLarge,
			fmt.Sprintf("pageSize %d exceeds max %d", pageSize, readdirMaxPageSize),
			false)
	}
	if args.Page < 0 {
		return errResp(req.ID, protocol.ErrCodeEINVAL,
			"page must be non-negative", false)
	}

	// Confirm the path is actually a directory before reading; os.ReadDir
	// on a regular file returns a confusing "not a directory" string that
	// differs across platforms, so we check up-front to produce EINVAL.
	info, err := os.Lstat(cleaned)
	if err != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
	}
	if !info.IsDir() {
		return errResp(req.ID, protocol.ErrCodeEINVAL,
			"not a directory: "+cleaned, false)
	}

	raw, err := os.ReadDir(cleaned)
	if err != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
	}

	entries := make([]readdirEntry, 0, len(raw))
	for _, de := range raw {
		// Obey context cancellation between entries so the host can
		// bail out promptly if the extension disconnects mid-scan on a
		// huge directory.
		select {
		case <-ctx.Done():
			return errResp(req.ID, protocol.ErrCodeCanceled,
				"readdir canceled", false)
		default:
		}

		e, ok := buildEntry(cleaned, de)
		if !ok {
			continue
		}
		if !args.IncludeHidden && e.Hidden {
			continue
		}
		entries = append(entries, e)
	}

	sortEntries(entries, args.Sort)

	total := len(entries)
	offset := args.Page * pageSize
	var pageEntries []readdirEntry
	var nextCursor *string
	if offset >= total {
		pageEntries = []readdirEntry{}
	} else {
		end := offset + pageSize
		if end > total {
			end = total
		}
		pageEntries = entries[offset:end]
		if end < total {
			// PROTOCOL.md §7.3 treats nextCursor as opaque; we use the
			// next page number as a stringified scalar so extensions
			// can pass it straight back as `cursor` (or `page`).
			next := fmt.Sprintf("%d", args.Page+1)
			nextCursor = &next
		}
	}

	return protocol.Response{
		ID: req.ID,
		OK: true,
		Data: readdirData{
			Entries:    pageEntries,
			NextCursor: nextCursor,
			Total:      total,
		},
	}
}

// buildEntry transforms a fs.DirEntry into our wire shape. Returns ok=false
// when the entry's Info() call fails so the caller can skip it without
// aborting the whole listing (one bad inode shouldn't hide the rest).
func buildEntry(parent string, de fs.DirEntry) (readdirEntry, bool) {
	name := de.Name()
	full := filepath.Join(parent, name)

	info, err := de.Info()
	if err != nil {
		return readdirEntry{}, false
	}

	isSymlink := de.Type()&fs.ModeSymlink != 0
	e := readdirEntry{
		Name:       name,
		Path:       full,
		ModifiedTs: info.ModTime().UnixMilli(),
		ReadOnly:   info.Mode().Perm()&0o200 == 0,
		Hidden:     isHidden(name, info),
		Symlink:    isSymlink,
	}
	switch {
	case isSymlink:
		e.Type = "symlink"
	case de.IsDir():
		e.Type = "directory"
	default:
		e.Type = "file"
	}
	// Only files carry a concrete size. For directories the number is
	// meaningless at this layer (Phase 1 does not recurse), so we emit JSON
	// null — see readdirEntry.SizeBytes docstring.
	if e.Type == "file" {
		sz := info.Size()
		e.SizeBytes = &sz
	}
	return e, true
}

// isHidden returns true when the entry should be filtered out with the
// default `includeHidden: false`. We consider two sources:
//
//	Unix: names starting with "." (POSIX convention)
//	Windows: FILE_ATTRIBUTE_HIDDEN flag via the underlying Win32FileAttributeData
//
// On macOS both rules coexist (filesystems also honour the UF_HIDDEN flag,
// which would require stat(2) — Phase 1 skips that for simplicity).
func isHidden(name string, info os.FileInfo) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if runtime.GOOS == "windows" {
		return hasWindowsHiddenAttr(info)
	}
	return false
}

// sortEntries applies args.Sort in-place. "directories first" is a common
// file-manager convention but the protocol only specifies the enum field
// set, not ordering semantics across type categories, so we keep the sort
// purely on the chosen key.
func sortEntries(entries []readdirEntry, s readdirSort) {
	field := s.Field
	if field == "" {
		field = "name"
	}
	desc := s.Order == "desc"

	less := func(i, j int) bool {
		a, b := entries[i], entries[j]
		switch field {
		case "size":
			av, bv := sizeForSort(a), sizeForSort(b)
			if av == bv {
				return a.Name < b.Name
			}
			return av < bv
		case "modified":
			if a.ModifiedTs == b.ModifiedTs {
				return a.Name < b.Name
			}
			return a.ModifiedTs < b.ModifiedTs
		case "type":
			if a.Type == b.Type {
				return a.Name < b.Name
			}
			return a.Type < b.Type
		default: // "name" or unknown — fall back to lexical
			// Case-insensitive primary + case-sensitive tiebreak so
			// "readme" and "README" don't compare as equal (which would
			// make sort.Sort unstable across Go versions).
			al, bl := strings.ToLower(a.Name), strings.ToLower(b.Name)
			if al == bl {
				return a.Name < b.Name
			}
			return al < bl
		}
	}
	if desc {
		// Swap operands rather than negate: negation breaks sort stability
		// when the comparator returns false on both (i,j) and (j,i) for
		// equal keys, producing spurious reorderings of tied entries.
		sort.SliceStable(entries, func(i, j int) bool { return less(j, i) })
	} else {
		sort.SliceStable(entries, less)
	}
}

// sizeForSort returns a comparable int64 for size sorting, treating nil
// (directories) as 0. This keeps directories adjacent in the sort output,
// which is what most file managers do.
func sizeForSort(e readdirEntry) int64 {
	if e.SizeBytes == nil {
		return 0
	}
	return *e.SizeBytes
}

// errResp is a shorthand for building a failure Response inline.
func errResp(id, code, msg string, retryable bool) protocol.Response {
	return protocol.Response{
		ID:    id,
		OK:    false,
		Error: protocol.NewError(code, msg, retryable),
	}
}
