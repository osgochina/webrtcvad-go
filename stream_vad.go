package webrtcvad

import (
	"errors"
	"time"
)

// stream_vad.go 提供流式VAD处理接口
// 自动处理缓冲和分帧，适合实时流处理场景

// StreamVAD 流式VAD处理器
type StreamVAD struct {
	vad        *VAD
	sampleRate int
	frameMs    int // 帧长度（毫秒）

	buffer     []byte // 缓冲区
	frameSize  int    // 单帧字节数
	segments   []VoiceSegment
	totalBytes int64 // 已处理的总字节数
}

// VoiceSegment 语音片段
type VoiceSegment struct {
	Start    time.Duration // 开始时间
	End      time.Duration // 结束时间
	IsSpeech bool          // 是否为语音
}

// NewStreamVAD 创建流式VAD处理器
//
// 参数:
//   - mode: VAD模式（0-3）
//   - sampleRate: 采样率（8000, 16000, 32000, 48000）
//   - frameMs: 帧长度（毫秒，10/20/30）
//
// 返回:
//   - *StreamVAD: 流式VAD实例
//   - error: 错误信息
func NewStreamVAD(mode int, sampleRate int, frameMs int) (*StreamVAD, error) {
	// 验证参数
	if !isValidSampleRate(sampleRate) {
		return nil, errors.New("invalid sample rate")
	}
	if frameMs != 10 && frameMs != 20 && frameMs != 30 {
		return nil, errors.New("frame length must be 10, 20, or 30 ms")
	}

	// 创建VAD实例
	vad, err := New(mode)
	if err != nil {
		return nil, err
	}

	// 计算帧大小（字节）
	frameSize := sampleRate * frameMs / 1000 * 2 // 16位 = 2字节

	return &StreamVAD{
		vad:        vad,
		sampleRate: sampleRate,
		frameMs:    frameMs,
		buffer:     make([]byte, 0, frameSize*2),
		frameSize:  frameSize,
		segments:   make([]VoiceSegment, 0, 100),
		totalBytes: 0,
	}, nil
}

// Write 写入音频数据，返回新检测到的语音片段
//
// 参数:
//   - data: 音频数据（16位PCM，小端序）
//
// 返回:
//   - []VoiceSegment: 新检测到的语音片段
//   - error: 错误信息
func (s *StreamVAD) Write(data []byte) ([]VoiceSegment, error) {
	// 将数据添加到缓冲区
	s.buffer = append(s.buffer, data...)

	var newSegments []VoiceSegment
	processed := 0

	// 处理所有完整的帧
	for len(s.buffer)-processed >= s.frameSize {
		frame := s.buffer[processed : processed+s.frameSize]

		// 检测当前帧
		isSpeech, err := s.vad.IsSpeech(frame, s.sampleRate)
		if err != nil {
			s.compactBuffer(processed)
			return nil, err
		}

		// 计算时间戳
		startTime := s.bytesToDuration(s.totalBytes)
		s.totalBytes += int64(s.frameSize)
		endTime := s.bytesToDuration(s.totalBytes)

		// 创建片段
		segment := VoiceSegment{
			Start:    startTime,
			End:      endTime,
			IsSpeech: isSpeech,
		}

		// 合并连续的相同类型片段
		if len(s.segments) > 0 {
			lastSegment := &s.segments[len(s.segments)-1]
			if lastSegment.IsSpeech == isSpeech {
				// 扩展最后一个片段
				lastSegment.End = endTime
			} else {
				// 添加新片段
				s.segments = append(s.segments, segment)
				newSegments = append(newSegments, segment)
			}
		} else {
			// 第一个片段
			s.segments = append(s.segments, segment)
			newSegments = append(newSegments, segment)
		}

		processed += s.frameSize
	}

	s.compactBuffer(processed)

	return newSegments, nil
}

func (s *StreamVAD) compactBuffer(processed int) {
	if processed == 0 {
		return
	}

	remaining := len(s.buffer) - processed
	copy(s.buffer, s.buffer[processed:])
	s.buffer = s.buffer[:remaining]
}

// GetSegments 获取所有语音片段
func (s *StreamVAD) GetSegments() []VoiceSegment {
	return s.segments
}

// Reset 重置流式VAD状态
func (s *StreamVAD) Reset() error {
	s.buffer = s.buffer[:0]
	s.segments = s.segments[:0]
	s.totalBytes = 0

	// 重新初始化VAD实例
	if err := initCore(s.vad.inst); err != nil {
		return err
	}

	return nil
}

// bytesToDuration 将字节数转换为时长
func (s *StreamVAD) bytesToDuration(bytes int64) time.Duration {
	// 字节 -> 样本 -> 秒 -> Duration
	samples := bytes / 2 // 16位 = 2字节
	seconds := float64(samples) / float64(s.sampleRate)
	return time.Duration(seconds * float64(time.Second))
}

// GetBufferSize 获取当前缓冲区大小（字节）
func (s *StreamVAD) GetBufferSize() int {
	return len(s.buffer)
}

// GetTotalProcessed 获取已处理的总字节数
func (s *StreamVAD) GetTotalProcessed() int64 {
	return s.totalBytes
}

// GetTotalDuration 获取已处理的总时长
func (s *StreamVAD) GetTotalDuration() time.Duration {
	return s.bytesToDuration(s.totalBytes)
}

// FilterSpeechSegments 过滤出语音片段
func (s *StreamVAD) FilterSpeechSegments() []VoiceSegment {
	var speech []VoiceSegment
	for _, seg := range s.segments {
		if seg.IsSpeech {
			speech = append(speech, seg)
		}
	}
	return speech
}

// FilterSilenceSegments 过滤出静音片段
func (s *StreamVAD) FilterSilenceSegments() []VoiceSegment {
	var silence []VoiceSegment
	for _, seg := range s.segments {
		if !seg.IsSpeech {
			silence = append(silence, seg)
		}
	}
	return silence
}
