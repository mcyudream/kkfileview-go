package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"kkfileview/internal/utils"
)

// SessionStore 简易会话存储（对应 HttpSession，用于删除验证码）
type SessionStore struct {
	mu   sync.RWMutex
	data map[string]*sessionData
}

type sessionData struct {
	captchaCode      string
	captchaGenSecond int64
}

// NewSessionStore 构造会话存储
func NewSessionStore() *SessionStore {
	s := &SessionStore{data: map[string]*sessionData{}}
	go s.cleanLoop()
	return s
}

func (s *SessionStore) cleanLoop() {
	for {
		time.Sleep(5 * time.Minute)
		s.mu.Lock()
		for k, v := range s.data {
			if utils.Dates.CalculateCurrentTimeDifference(v.captchaGenSecond) > 3600 {
				delete(s.data, k)
			}
		}
		s.mu.Unlock()
	}
}

// get 获取或创建会话
func (s *SessionStore) get(w http.ResponseWriter, r *http.Request) (*sessionData, string) {
	id := sessionID(r)
	if id == "" {
		id = newSessionID()
		http.SetCookie(w, &http.Cookie{
			Name:     "JSESSIONID",
			Value:    id,
			Path:     "/",
			HttpOnly: true,
		})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data[id]
	if !ok {
		d = &sessionData{}
		s.data[id] = d
	}
	return d, id
}

func sessionID(r *http.Request) string {
	c, err := r.Cookie("JSESSIONID")
	if err != nil {
		return ""
	}
	return c.Value
}

// captchaCode 读取会话中的验证码
func (s *SessionStore) captchaCode(r *http.Request) string {
	id := sessionID(r)
	if id == "" {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if d, ok := s.data[id]; ok {
		return d.captchaCode
	}
	return ""
}

func newSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// handleDeleteFileCaptcha 对应 FileController.captcha
func (s *Server) handleDeleteFileCaptcha(w http.ResponseWriter, r *http.Request) {
	if !configGetDeleteCaptcha() {
		return
	}
	sess, _ := s.sessions.get(w, r)
	s.sessions.mu.RLock()
	captchaCode := sess.captchaCode
	captchaGenerateTime := sess.captchaGenSecond
	s.sessions.mu.RUnlock()
	timeDifference := utils.Dates.CalculateCurrentTimeDifference(captchaGenerateTime)

	if timeDifference > 50 && captchaCode == "" {
		code := utils.Captcha.GenerateCaptchaCode()
		s.sessions.mu.Lock()
		sess.captchaCode = code
		sess.captchaGenSecond = utils.Dates.GetCurrentSecond()
		s.sessions.mu.Unlock()
		captchaCode = code
	} else {
		if captchaCode == "" {
			captchaCode = "wait"
		}
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Expires", "-1")
	w.Write(utils.Captcha.GenerateCaptchaPic(captchaCode))
}
