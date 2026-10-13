package utils

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"kkfileview/internal/config"
)

// cfgFtpUsername 读取 ftp.username 配置
func cfgFtpUsername() string {
	return config.Get().FtpUsername
}

// FtpUtils FTP 工具（stdlib net 实现，对应 Java FtpUtils）
type ftpUtils struct{}

// FtpUtils 导出单例
var FtpUtils = &ftpUtils{}

type ftpConn struct {
	conn         net.Conn
	r            *bufio.Reader
	lastCode     string
	lastDataAddr string
}

// Download 下载 FTP 文件到本地
func (f *ftpUtils) Download(urlStr, portStr, localPath, username, password, controlEncoding string) error {
	u, err := parseURL(urlStr)
	if err != nil {
		return err
	}
	username, password, controlEncoding = resolveFtpCredentials(u, username, password, controlEncoding)
	addr := net.JoinHostPort(u.host, portOfFtp(u, portStr))
	c, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Minute))
	fc := &ftpConn{conn: c, r: bufio.NewReader(c)}
	if err := fc.readLine(); err != nil {
		return err
	}
	if err := fc.cmd("USER " + orEmpty(username, "anonymous")); err != nil {
		return err
	}
	if password != "" {
		if err := fc.cmd("PASS " + password); err != nil {
			return err
		}
	}
	if err := fc.cmd("TYPE I"); err != nil {
		return err
	}
	if err := fc.cmd("PASV"); err != nil {
		return err
	}
	dataAddr := fc.lastDataAddr
	dc, err := net.DialTimeout("tcp", dataAddr, 30*time.Second)
	if err != nil {
		return err
	}
	defer dc.Close()
	dc.SetDeadline(time.Now().Add(10 * time.Minute))
	if err := fc.cmd("RETR " + u.path); err != nil {
		return fmt.Errorf("FTP RETR 失败")
	}
	if err := os.MkdirAll(dirOf(localPath), 0755); err != nil {
		return err
	}
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, dc)
	if err != nil {
		return err
	}
	fc.readLine()
	return fc.cmd("QUIT")
}

// Preview 对应 Java FtpUtils.preview：打开 FTP 文件流
func (f *ftpUtils) Preview(urlStr, portStr, filename, username, password, controlEncoding string) (io.ReadCloser, error) {
	u, err := parseURL(urlStr)
	if err != nil {
		return nil, err
	}
	username, password, controlEncoding = resolveFtpCredentials(u, username, password, controlEncoding)
	addr := net.JoinHostPort(u.host, portOfFtp(u, portStr))
	c, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Minute))
	fc := &ftpConn{conn: c, r: bufio.NewReader(c)}
	if err := fc.readLine(); err != nil {
		return nil, err
	}
	if err := fc.cmd("USER " + orEmpty(username, "anonymous")); err != nil {
		return nil, err
	}
	if password != "" {
		if err := fc.cmd("PASS " + password); err != nil {
			return nil, err
		}
	}
	if err := fc.cmd("TYPE I"); err != nil {
		return nil, err
	}
	if err := fc.cmd("PASV"); err != nil {
		return nil, err
	}
	dc, err := net.DialTimeout("tcp", fc.lastDataAddr, 30*time.Second)
	if err != nil {
		return nil, err
	}
	defer dc.Close()
	dc.SetDeadline(time.Now().Add(10 * time.Minute))
	if err := fc.cmd("RETR " + u.path); err != nil {
		return nil, fmt.Errorf("FTP RETR 失败")
	}
	return dc, nil
}

func orEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func dirOf(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i > 0 {
		return p[:i]
	}
	return "."
}

type ftpURL struct {
	host string
	path string
}

func parseURL(s string) (*ftpURL, error) {
	if !strings.HasPrefix(strings.ToLower(s), "ftp://") {
		return nil, fmt.Errorf("非 FTP URL: %s", s)
	}
	rest := s[len("ftp://"):]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return &ftpURL{host: rest, path: "/"}, nil
	}
	return &ftpURL{host: rest[:slash], path: rest[slash:]}, nil
}

// resolveFtpCredentials 从 ftp.username 配置中解析该主机的凭据
func resolveFtpCredentials(u *ftpURL, username, password, controlEncoding string) (string, string, string) {
	cfg := cfgFtpUsername()
	if cfg == "" || cfg == "false" {
		return username, password, controlEncoding
	}
	// 格式: 地址:端口:用户名:密码:编码,多客户端逗号分隔
	for _, item := range strings.Split(cfg, ",") {
		item = strings.TrimSpace(item)
		parts := strings.Split(item, ":")
		if len(parts) >= 3 && parts[0] == u.host {
			enc := controlEncoding
			if len(parts) >= 4 {
				enc = parts[len(parts)-1]
			}
			return parts[1], parts[2], enc
		}
	}
	return username, password, controlEncoding
}

func portOfFtp(u *ftpURL, portStr string) string {
	if portStr != "" {
		return portStr
	}
	if strings.Contains(u.host, ":") {
		parts := strings.SplitN(u.host, ":", 2)
		u.host = parts[0]
		return parts[1]
	}
	return "21"
}

func (fc *ftpConn) readLine() error {
	line, err := fc.r.ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	fc.lastCode = line
	if len(line) < 3 || line[0] < '1' || line[0] > '5' {
		return fmt.Errorf("FTP 错误响应: %s", strings.TrimSpace(line))
	}
	if line[0] == '4' || line[0] == '5' {
		if strings.HasPrefix(line, "550") {
			return fmt.Errorf("ftp 550: 文件不存在")
		}
		return fmt.Errorf("FTP 错误响应: %s", strings.TrimSpace(line))
	}
	// PASV 响应解析数据地址: 227 Entering Passive Mode (h1,h2,h3,h4,p1,p2).
	if strings.HasPrefix(line, "227") {
		open := strings.IndexByte(line, '(')
		close := strings.IndexByte(line, ')')
		if open >= 0 && close > open {
			var h1, h2, h3, h4, p1, p2 int
			fmt.Sscanf(line[open+1:close], "%d,%d,%d,%d,%d,%d", &h1, &h2, &h3, &h4, &p1, &p2)
			fc.lastDataAddr = fmt.Sprintf("%d.%d.%d.%d:%d", h1, h2, h3, h4, p1*256+p2)
		}
	}
	return nil
}

func (fc *ftpConn) cmd(command string) error {
	if _, err := fc.conn.Write([]byte(command + "\r\n")); err != nil {
		return err
	}
	return fc.readLine()
}
