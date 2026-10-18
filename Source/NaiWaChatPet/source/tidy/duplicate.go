package tidy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type DuplicateReport struct {
	Groups    [][]string
	Skipped   []Skip
	BytesRead int64
}

// FindDuplicates is opt-in and local only. It compares stable candidate files under fixed IO budgets.
func FindDuplicates(ctx context.Context, p Plan) (DuplicateReport, error) {
	r := DuplicateReport{}
	bySize := map[int64][]Move{}
	for _, m := range p.Moves {
		bySize[p.Facts[m.From].Size] = append(bySize[p.Facts[m.From].Size], m)
	}
	var sizes []int64
	for size := range bySize {
		sizes = append(sizes, size)
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })
	hashes := map[string][]string{}
	for _, size := range sizes {
		files := bySize[size]
		if len(files) < 2 {
			continue
		}
		for _, m := range files {
			if e := ctx.Err(); e != nil {
				return r, e
			}
			if size > 64<<20 || r.BytesRead+size > 256<<20 {
				r.Skipped = append(r.Skipped, Skip{filepath.Base(m.From), "超出单文件 64 MiB / 单次 256 MiB 读取预算"})
				continue
			}
			id, e := identify(m.From)
			if e != nil || id != p.Facts[m.From] {
				r.Skipped = append(r.Skipped, Skip{filepath.Base(m.From), "文件已变化或受保护"})
				continue
			}
			f, e := os.Open(m.From)
			if e != nil {
				r.Skipped = append(r.Skipped, Skip{filepath.Base(m.From), "无法读取"})
				continue
			}
			h := sha256.New()
			buf := make([]byte, 64<<10)
			var total int64
			for {
				if e = ctx.Err(); e != nil {
					break
				}
				var n int
				n, e = f.Read(buf)
				if n > 0 {
					total += int64(n)
					r.BytesRead += int64(n)
					h.Write(buf[:n])
				}
				if total > size {
					e = errors.New("文件长度变化")
					break
				}
				if e != nil {
					break
				}
			}
			f.Close()
			after, ae := identify(m.From)
			if e != io.EOF || ae != nil || after != id || total != size {
				r.Skipped = append(r.Skipped, Skip{filepath.Base(m.From), "读取中变化，校验作废"})
				continue
			}
			key := hex.EncodeToString(h.Sum(nil))
			hashes[key] = append(hashes[key], filepath.Base(m.From))
		}
	}
	for _, names := range hashes {
		if len(names) > 1 {
			sort.Strings(names)
			r.Groups = append(r.Groups, names)
		}
	}
	sort.Slice(r.Groups, func(i, j int) bool { return r.Groups[i][0] < r.Groups[j][0] })
	return r, nil
}
