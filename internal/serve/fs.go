package serve

// fs.go 是工作台「工作目录」选择器的后端：GET /api/fs/dirs 列出一个目录下
// 的子目录，供前端点选。之前这一栏只能手打绝对路径，而用户脑子里通常只有
// 「大概在 D 盘某个项目目录」，打错一个字就要重来。
//
// 只读、只列目录、只在 -rw 下可用：工作目录是写模式的概念（它决定这一轮
// 成员在哪个目录里跑），只读看板没有这一栏。列目录不读文件内容、不判断
// 权限之外的东西——它不是文件浏览器，只是选目录。

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// fsDirEntry 是选择器里的一行。
type fsDirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// fsDirsResponse 是一次列目录的结果。Parent 为空表示已经在根上（Windows 的
// 盘符列表或 POSIX 的 /），前端据此把「上一级」画成禁用。
type fsDirsResponse struct {
	Path    string       `json:"path"`
	Parent  string       `json:"parent"`
	Entries []fsDirEntry `json:"entries"`
	// Roots 只在 path 为空时给：Windows 是存在的盘符，其它系统是 /。
	Roots []fsDirEntry `json:"roots"`
}

// handleFSDirs 列目录。path 为空时回工作目录本身与根列表，供选择器开屏。
func (s *Server) handleFSDirs(w http.ResponseWriter, r *http.Request) {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有目录接口：用 council serve -rw 启动才能选工作目录")
		return
	}
	if !requireGET(w, r) {
		return
	}

	raw := strings.TrimSpace(r.URL.Query().Get("path"))
	if raw == "" {
		raw = s.workbenchCwd()
	}
	if raw == "" {
		writeError(w, http.StatusBadRequest, "没有可列的目录：启动目录未知，请显式给出 path")
		return
	}
	if !filepath.IsAbs(raw) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("path 必须是绝对路径：%q", raw))
		return
	}
	clean := filepath.Clean(raw)
	info, err := os.Stat(clean)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeError(w, http.StatusBadRequest, "目录不存在："+clean)
			return
		}
		writeError(w, http.StatusBadRequest, "目录不可用："+clean+"："+err.Error())
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "不是目录："+clean)
		return
	}

	entries, err := listSubdirs(clean)
	if err != nil {
		// 权限不足是这里最常见的一种：别把它说成服务端错误，用户还能去
		// 上一层选别的目录。
		writeError(w, http.StatusBadRequest, "读不了这个目录："+clean+"："+err.Error())
		return
	}

	resp := fsDirsResponse{Path: clean, Parent: parentDir(clean), Entries: entries, Roots: volumeRoots()}
	writeJSON(w, http.StatusOK, resp)
}

// listSubdirs 只列目录，按名字排序（大小写不敏感，Windows 资源管理器的习惯）。
// 不跟随符号链接：指来指去的链接会让「上一级」原地打转。
func listSubdirs(dir string) ([]fsDirEntry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]fsDirEntry, 0, len(items))
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		entries = append(entries, fsDirEntry{
			Name: item.Name(),
			Path: filepath.Join(dir, item.Name()),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

// parentDir 给出上一级；已经在根上时回空串。
func parentDir(dir string) string {
	parent := filepath.Dir(dir)
	if parent == dir {
		return ""
	}
	return parent
}

// volumeRoots 是选择器开屏的根列表。Windows 上只列真实存在的盘符。
func volumeRoots() []fsDirEntry {
	if filepath.Separator != '\\' {
		return []fsDirEntry{{Name: "/", Path: "/"}}
	}
	roots := make([]fsDirEntry, 0, 4)
	for letter := 'A'; letter <= 'Z'; letter++ {
		path := string(letter) + `:\`
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			roots = append(roots, fsDirEntry{Name: string(letter) + ":", Path: path})
		}
	}
	return roots
}
