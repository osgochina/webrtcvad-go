// Package webrtcvad 提供Go语言实现的WebRTC语音活动检测(VAD)功能
//
// 这是Google WebRTC项目中VAD模块的纯Go移植版本，无需cgo或外部依赖。
// VAD(Voice Activity Detection)用于检测音频帧中是否包含语音。
//
// 使用示例:
//
//	vad, err := webrtcvad.New(1) // 创建VAD实例，激进度模式1
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 检测16位PCM音频（16kHz采样率，10ms帧长）
//	isSpeech, err := vad.IsSpeech(audioData, 16000)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	if isSpeech {
//	    fmt.Println("检测到语音")
//	}
package webrtcvad

import (
	"errors"
	"fmt"
)

// VAD 语音活动检测器
type VAD struct {
	inst         *vadInst
	sampleBuffer []int16
}

// New 创建一个新的VAD实例
//
// mode 参数控制检测的激进程度：
//   - 0: 质量模式（最不激进，更容易检测到语音）
//   - 1: 低比特率模式
//   - 2: 激进模式
//   - 3: 非常激进模式（最激进，更严格的语音判定）
//
// 激进度越高，对语音的判定越严格，误检率降低但可能漏检语音。
func New(mode int) (*VAD, error) {
	if mode < 0 || mode > 3 {
		return nil, fmt.Errorf("mode must be 0-3, got %d", mode)
	}

	inst := createVadInst()
	if err := initCore(inst); err != nil {
		return nil, fmt.Errorf("failed to initialize VAD: %w", err)
	}

	if err := setModeCore(inst, mode); err != nil {
		return nil, fmt.Errorf("failed to set mode: %w", err)
	}

	return &VAD{inst: inst}, nil
}

// SetMode 设置VAD的激进度模式
//
// mode 参数范围：0-3（含义见New函数说明）
func (v *VAD) SetMode(mode int) error {
	if mode < 0 || mode > 3 {
		return fmt.Errorf("mode must be 0-3, got %d", mode)
	}

	if v.inst.initFlag != kInitCheck {
		return errors.New("VAD not initialized")
	}

	return setModeCore(v.inst, mode)
}

// IsSpeech 检测音频帧中是否包含语音
//
// 参数:
//   - buf: 16位小端序PCM音频数据（字节数组）
//   - sampleRate: 采样率，必须是8000, 16000, 32000或48000 Hz
//
// 返回:
//   - bool: true表示检测到语音，false表示静音或噪声
//   - error: 如果参数无效或处理失败
//
// 注意：
//   - 音频帧长度必须是10ms、20ms或30ms
//   - buf长度应该是 (sampleRate * frameDurationMs / 1000) * 2 字节
func (v *VAD) IsSpeech(buf []byte, sampleRate int) (bool, error) {
	// 将字节数组转换为int16数组
	audioFrame := v.bytesToInt16(buf)

	return v.IsSpeechInt16(audioFrame, sampleRate)
}

// IsSpeechInt16 检测int16音频帧中是否包含语音。
//
// 参数:
//   - audioFrame: 16位PCM音频数据（样本数组）
//   - sampleRate: 采样率，必须是8000, 16000, 32000或48000 Hz
func (v *VAD) IsSpeechInt16(audioFrame []int16, sampleRate int) (bool, error) {
	if v.inst.initFlag != kInitCheck {
		return false, errors.New("VAD not initialized")
	}

	// 验证采样率
	if !isValidSampleRate(sampleRate) {
		return false, fmt.Errorf("invalid sample rate: %d (must be 8000, 16000, 32000, or 48000)", sampleRate)
	}

	// 计算帧长度（样本数）
	frameLength := len(audioFrame)

	// 验证帧长度
	if !ValidRateAndFrameLength(sampleRate, frameLength) {
		return false, fmt.Errorf("invalid frame length %d for sample rate %d", frameLength, sampleRate)
	}

	// 处理音频并返回VAD决策
	vad, err := process(v.inst, sampleRate, audioFrame)
	if err != nil {
		return false, err
	}

	return vad > 0, nil
}

// ValidRateAndFrameLength 检查采样率和帧长度的组合是否有效
//
// 参数:
//   - rate: 采样率（Hz）
//   - frameLength: 帧长度（样本数）
//
// 返回:
//   - bool: true表示组合有效
//
// 有效的组合：
//   - 采样率必须是8000, 16000, 32000或48000 Hz
//   - 帧长度必须对应10ms、20ms或30ms
func ValidRateAndFrameLength(rate, frameLength int) bool {
	validRates := []int{8000, 16000, 32000, 48000}
	maxFrameLengthMs := 30

	// 检查采样率是否在有效列表中
	for _, r := range validRates {
		if r == rate {
			// 检查帧长度（10ms、20ms或30ms）
			for ms := 10; ms <= maxFrameLengthMs; ms += 10 {
				expectedLength := rate * ms / 1000
				if frameLength == expectedLength {
					return true
				}
			}
			break
		}
	}

	return false
}

// 辅助函数：检查采样率是否有效
func isValidSampleRate(rate int) bool {
	return rate == 8000 || rate == 16000 || rate == 32000 || rate == 48000
}

// IsSpeechBatch 批量检测多个音频帧
//
// 参数:
//   - frames: 音频帧数组，每个元素是一帧的PCM数据
//   - sampleRate: 采样率
//
// 返回:
//   - []bool: 每一帧的检测结果
//   - error: 错误信息
func (v *VAD) IsSpeechBatch(frames [][]byte, sampleRate int) ([]bool, error) {
	results := make([]bool, len(frames))

	for i, frame := range frames {
		isSpeech, err := v.IsSpeech(frame, sampleRate)
		if err != nil {
			return results, fmt.Errorf("frame %d: %w", i, err)
		}
		results[i] = isSpeech
	}

	return results, nil
}

// IsSpeechBatchTo 批量检测（零分配版本）
//
// 参数:
//   - frames: 音频帧数组
//   - sampleRate: 采样率
//   - results: 预分配的结果数组（长度应 >= len(frames)）
//
// 返回:
//   - error: 错误信息
//
// 注意：此函数使用预分配的results数组，避免内存分配
func (v *VAD) IsSpeechBatchTo(frames [][]byte, sampleRate int, results []bool) error {
	if len(results) < len(frames) {
		return errors.New("results array too small")
	}

	for i, frame := range frames {
		isSpeech, err := v.IsSpeech(frame, sampleRate)
		if err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
		results[i] = isSpeech
	}

	return nil
}

// 辅助函数：将字节数组转换为int16数组（小端序）
func (v *VAD) bytesToInt16(buf []byte) []int16 {
	length := len(buf) / 2
	if cap(v.sampleBuffer) < length {
		v.sampleBuffer = make([]int16, length)
	}
	result := v.sampleBuffer[:length]

	for i := 0; i < length; i++ {
		// 小端序：低字节在前
		result[i] = int16(buf[i*2]) | (int16(buf[i*2+1]) << 8)
	}

	return result
}
