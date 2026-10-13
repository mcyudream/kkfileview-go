package utils

import (
	"sync"
	"time"
)

// Status 转换状态
type Status int

const (
	StatusConverting Status = iota
	StatusTimeout
	StatusFailed
	StatusSuccess
)

// ConvertStatus 转换状态快照（对应 Java ConvertStatus）
type ConvertStatus struct {
	Status         Status
	Message        string
	Progress       int
	WaitingSeconds int64
}

type convertState struct {
	status    Status
	message   string
	progress  int
	startTime int64
}

// FileConvertStatusManager 对应 Java FileConvertStatusManager
type FileConvertStatusManager struct{}

// StatusManager 导出单例
var StatusManager = &FileConvertStatusManager{}

var (
	statusMu    sync.RWMutex
	statusMap   = map[string]*convertState{}
	finalStatus = map[string]Status{}
)

func (m *FileConvertStatusManager) StartConvert(fileName string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	if _, ok := statusMap[fileName]; !ok {
		statusMap[fileName] = &convertState{status: StatusConverting, message: "等待转换", progress: 0, startTime: time.Now().UnixMilli()}
	}
	delete(finalStatus, fileName)
}

func (m *FileConvertStatusManager) UpdateProgress(fileName, message string, progress int) {
	statusMu.Lock()
	defer statusMu.Unlock()
	if st, ok := statusMap[fileName]; ok {
		st.message = message
		st.progress = progress
	}
}

func (m *FileConvertStatusManager) MarkTimeout(fileName string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	statusMap[fileName] = &convertState{status: StatusTimeout, message: "转换超时，请重试", progress: 0}
	finalStatus[fileName] = StatusTimeout
}

func (m *FileConvertStatusManager) MarkError(fileName, errorMessage string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	statusMap[fileName] = &convertState{status: StatusFailed, message: errorMessage, progress: 0}
	finalStatus[fileName] = StatusFailed
}

func (m *FileConvertStatusManager) GetConvertStatus(fileName string) *ConvertStatus {
	statusMu.RLock()
	defer statusMu.RUnlock()
	if fs, ok := finalStatus[fileName]; ok && (fs == StatusTimeout || fs == StatusFailed) {
		if st, ok := statusMap[fileName]; ok {
			return &ConvertStatus{Status: st.status, Message: st.message, Progress: st.progress}
		}
		if fs == StatusTimeout {
			return &ConvertStatus{Status: StatusTimeout, Message: "转换超时，请重试"}
		}
		return &ConvertStatus{Status: StatusFailed, Message: "转换失败"}
	}
	st, ok := statusMap[fileName]
	if !ok {
		return nil
	}
	var waiting int64
	if st.status == StatusConverting {
		waiting = (time.Now().UnixMilli() - st.startTime) / 1000
	}
	return &ConvertStatus{Status: st.status, Message: st.message, Progress: st.progress, WaitingSeconds: waiting}
}

func (m *FileConvertStatusManager) ConvertSuccess(fileName string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	delete(statusMap, fileName)
	delete(finalStatus, fileName)
}

// ClearStatus 强制重置状态
func (m *FileConvertStatusManager) ClearStatus(fileName string) bool {
	statusMu.Lock()
	defer statusMu.Unlock()
	_, a := statusMap[fileName]
	_, b := finalStatus[fileName]
	delete(statusMap, fileName)
	delete(finalStatus, fileName)
	return a || b
}

// RealTimeMessage 实时消息（message + waiting）
func (s *ConvertStatus) RealTimeMessage() string {
	return s.Message
}
