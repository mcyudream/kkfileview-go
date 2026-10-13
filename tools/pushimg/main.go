// pushimg 以「分块上传」方式把本地 docker save 出来的 OCI 镜像推送到 registry v2。
// 存在意义：该私有仓库前置 nginx 的 client_max_body_size 约 1MB，
// docker push 的单体 PUT 会被 413 拒绝；改用 512KB PATCH 分块即可绕过。
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const chunkSize = 512 * 1024

type descriptor struct {
	MediaType string     `json:"mediaType"`
	Digest    string     `json:"digest"`
	Size      int64      `json:"size"`
	Platform  *platformT `json:"platform,omitempty"`
}

type platformT struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

type indexDoc struct {
	Manifests []descriptor `json:"manifests"`
}

type imgManifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Config        descriptor   `json:"config"`
	Layers        []descriptor `json:"layers"`
}

type client struct {
	base      string
	repo      string
	tag       string
	mountFrom string
	mountDig  string
	user      string
	pass      string
	hc        *http.Client
}

func main() {
	var (
		dir       = flag.String("dir", "", "docker save 解包后的 OCI 目录（含 index.json 与 blobs/）")
		server    = flag.String("server", "https://registry.yudream.online", "registry 地址")
		repo      = flag.String("repo", "", "目标仓库，如 library/kkfileview-go")
		tag       = flag.String("tag", "latest", "目标 tag")
		mountFrom = flag.String("mount-from", "", "跨仓库 mount 的来源摘要，如 <digest>；来源仓库取 -mount-src")
		mountSrc  = flag.String("mount-src", "", "mount 来源仓库名，如 library/kkfileview（需与目标同项目）")
	)
	flag.Parse()

	user, pass := os.Getenv("REG_USER"), os.Getenv("REG_PASS")
	if *repo == "" || *dir == "" || user == "" || pass == "" {
		fmt.Fprintln(os.Stderr, "用法: pushimg -dir <oci目录> -repo library/kkfileview-go -tag latest [-mount-src library/kkfileview]")
		os.Exit(2)
	}

	c := &client{
		base:      strings.TrimRight(*server, "/"),
		repo:      *repo,
		tag:       *tag,
		mountFrom: *mountSrc,
		mountDig:  *mountFrom,
		user:      user,
		pass:      pass,
		hc:        &http.Client{Timeout: 60 * time.Minute},
	}

	idxRaw, err := os.ReadFile(filepath.Join(*dir, "index.json"))
	must(err)
	var idx indexDoc
	must(json.Unmarshal(idxRaw, &idx))
	if len(idx.Manifests) == 0 {
		fatal("index.json 中没有 manifests")
	}

	blobPath := func(digest string) string {
		algo, hexPart := splitDigest(digest)
		return filepath.Join(*dir, "blobs", algo, hexPart)
	}

	// index.json 的条目可能仍是 index（内嵌镜像 manifest + attestation），逐层解析到镜像 manifest
	mfDesc := idx.Manifests[0]
	for i := 0; i < 4; i++ {
		raw, err := os.ReadFile(blobPath(mfDesc.Digest))
		must(err)
		var probe struct {
			MediaType string       `json:"mediaType"`
			Manifests []descriptor `json:"manifests"`
		}
		must(json.Unmarshal(raw, &probe))
		if len(probe.Manifests) == 0 {
			break // 已是镜像 manifest
		}
		var next *descriptor
		for _, m := range probe.Manifests {
			if m.Platform != nil && m.Platform.OS == "linux" && m.Platform.Architecture == "amd64" {
				mm := m
				next = &mm
				break
			}
		}
		if next == nil {
			for _, m := range probe.Manifests {
				if m.Platform == nil || (m.Platform.Architecture != "unknown" && m.Platform.OS != "unknown") {
					mm := m
					next = &mm
					break
				}
			}
		}
		if next == nil {
			fatal("索引中未找到镜像 manifest")
		}
		mfDesc = *next
	}
	fmt.Printf("镜像 manifest: %s (%d bytes) %s\n", mfDesc.Digest, mfDesc.Size, mfDesc.MediaType)

	mfRaw, err := os.ReadFile(blobPath(mfDesc.Digest))
	must(err)
	var mf imgManifest
	must(json.Unmarshal(mfRaw, &mf))

	type blob struct {
		digest string
		path   string
	}
	blobs := []blob{{mfDesc.Digest, blobPath(mfDesc.Digest)}}
	blobs = append(blobs, blob{mf.Config.Digest, blobPath(mf.Config.Digest)})
	for _, l := range mf.Layers {
		blobs = append(blobs, blob{l.Digest, blobPath(l.Digest)})
	}

	for i, b := range blobs {
		fi, err := os.Stat(b.path)
		must(err)
		actual := "sha256:" + sha256File(b.path)
		if actual != b.digest {
			fatal("blob 摘要不匹配: %s != %s", actual, b.digest)
		}
		start := time.Now()
		status, err := c.pushBlob(b.digest, b.path, fi.Size())
		if err != nil {
			fatal("推送 %s 失败: %v", b.digest, err)
		}
		fmt.Printf("[%d/%d] %s (%4d MB) %-6s %.1fs\n", i+1, len(blobs), b.digest[:19], fi.Size()/1024/1024, status, time.Since(start).Seconds())
	}

	must(c.putManifest(mfRaw))
	fmt.Printf("完成: %s/%s:%s\n", c.base, c.repo, c.tag)
}

func (c *client) pushBlob(digest, path string, size int64) (string, error) {
	if c.headBlob(digest) {
		return "已存在", nil
	}
	// 尝试跨仓库 mount（同项目内且配置了来源时）
	if c.mountFrom != "" && c.mountDig != "" {
		if code, _ := c.startUpload(); code == http.StatusCreated {
			return "已挂载", nil
		}
	}
	code, loc := c.startUpload()
	if code != http.StatusAccepted || loc == "" {
		return "", fmt.Errorf("初始化上传失败 HTTP %d", code)
	}
	loc, uerr := c.uploadChunks(loc, path, size)
	if uerr != nil {
		return "", uerr
	}
	if err := c.commitUpload(loc, digest); err != nil {
		return "", err
	}
	return "已上传", nil
}

func (c *client) headBlob(digest string) bool {
	req, _ := http.NewRequest(http.MethodHead, fmt.Sprintf("%s/v2/%s/blobs/%s", c.base, c.repo, digest), nil)
	c.auth(req)
	resp, err := c.hc.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (c *client) startUpload() (int, string) {
	u := fmt.Sprintf("%s/v2/%s/blobs/uploads/", c.base, c.repo)
	if c.mountFrom != "" && c.mountDig != "" {
		u += "?mount=" + url.QueryEscape(c.mountDig) + "&from=" + url.QueryEscape(c.mountFrom)
	}
	req, _ := http.NewRequest(http.MethodPost, u, nil)
	c.auth(req)
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, ""
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Location")
}

func (c *client) uploadChunks(loc string, path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, chunkSize)
	offset := int64(0)
	for offset < size {
		n, err := f.Read(buf)
		if n > 0 {
			end := offset + int64(n) - 1
			req, _ := http.NewRequest(http.MethodPatch, loc, bytes.NewReader(buf[:n]))
			req.Header.Set("Content-Type", "application/octet-stream")
			req.Header.Set("Content-Range", fmt.Sprintf("%d-%d", offset, end))
			req.ContentLength = int64(n)
			c.auth(req)
			resp, err := c.hc.Do(req)
			if err != nil {
				return loc, err
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return loc, fmt.Errorf("PATCH 失败 HTTP %d", resp.StatusCode)
			}
			if nl := resp.Header.Get("Location"); nl != "" {
				loc = nl
			}
			offset += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return loc, err
		}
	}
	return loc, nil
}

func (c *client) commitUpload(loc, digest string) error {
	sep := "&"
	if !strings.Contains(loc, "?") {
		sep = "?"
	}
	req, _ := http.NewRequest(http.MethodPut, loc+sep+"digest="+url.QueryEscape(digest), nil)
	c.auth(req)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("提交上传失败 HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *client) putManifest(raw []byte) error {
	req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/v2/%s/manifests/%s", c.base, c.repo, c.tag), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	c.auth(req)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("推送 manifest 失败 HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *client) auth(req *http.Request) { req.SetBasicAuth(c.user, c.pass) }

func splitDigest(d string) (algo, hex string) {
	i := strings.Index(d, ":")
	if i < 0 {
		return "sha256", d
	}
	return d[:i], d[i+1:]
}

func sha256File(path string) string {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	must(err)
	return hex.EncodeToString(h.Sum(nil))
}

func must(err error) {
	if err != nil {
		fatal("%v", err)
	}
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "pushimg: "+format+"\n", a...)
	os.Exit(1)
}
