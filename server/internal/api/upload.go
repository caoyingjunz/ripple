package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// saveImage 校验并保存上传图片（仅图片、内容嗅探、≤2MB），返回 /uploads/xxx url
func (s *Server) saveImage(w http.ResponseWriter, r *http.Request) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, s.Cfg.MaxUpload+1024)
	if err := r.ParseMultipartForm(s.Cfg.MaxUpload); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "file too large")
		return "", false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "file required")
		return "", false
	}
	defer file.Close()
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	ctype := http.DetectContentType(buf[:n])
	var ext string
	switch ctype {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	default:
		writeErr(w, http.StatusBadRequest, "unsupported image type")
		return "", false
	}
	if err := os.MkdirAll(s.Cfg.UploadDir, 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return "", false
	}
	name := uuid.NewString() + ext
	path := filepath.Join(s.Cfg.UploadDir, name)
	out, err := os.Create(path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return "", false
	}
	defer out.Close()
	if _, err := out.Write(buf[:n]); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return "", false
	}
	if _, err := io.Copy(out, file); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return "", false
	}
	return "/uploads/" + name, true
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	url, ok := s.saveImage(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}
