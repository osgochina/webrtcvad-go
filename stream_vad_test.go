package webrtcvad

import (
	"testing"
	"time"
)

// TestStreamVADCreation 测试StreamVAD创建
func TestStreamVADCreation(t *testing.T) {
	// 测试有效参数
	svad, err := NewStreamVAD(1, 16000, 20)
	if err != nil {
		t.Fatalf("创建StreamVAD失败: %v", err)
	}
	if svad == nil {
		t.Fatal("StreamVAD实例为nil")
	}

	// 测试无效采样率
	_, err = NewStreamVAD(1, 11025, 20)
	if err == nil {
		t.Error("应该拒绝无效采样率")
	}

	// 测试无效帧长度
	_, err = NewStreamVAD(1, 16000, 15)
	if err == nil {
		t.Error("应该拒绝无效帧长度")
	}
}

// TestStreamVADWrite 测试流式写入
func TestStreamVADWrite(t *testing.T) {
	svad, err := NewStreamVAD(1, 16000, 20)
	if err != nil {
		t.Fatalf("创建StreamVAD失败: %v", err)
	}

	// 创建测试音频（20ms @16kHz = 320样本 = 640字节）
	frameSize := 16000 * 20 / 1000 * 2
	audioData := make([]byte, frameSize*3) // 3帧

	// 写入音频
	segments, err := svad.Write(audioData)
	if err != nil {
		t.Fatalf("写入音频失败: %v", err)
	}

	// 应该检测到3帧
	if len(segments) == 0 {
		t.Error("应该检测到至少1个片段")
	}

	// 检查总处理字节数
	if svad.GetTotalProcessed() != int64(frameSize*3) {
		t.Errorf("总处理字节数错误: 期望%d, 得到%d", frameSize*3, svad.GetTotalProcessed())
	}
}

// TestStreamVADBuffering 测试缓冲功能
func TestStreamVADBuffering(t *testing.T) {
	svad, err := NewStreamVAD(1, 8000, 10)
	if err != nil {
		t.Fatalf("创建StreamVAD失败: %v", err)
	}

	// 创建不完整帧（10ms @8kHz = 80样本 = 160字节）
	frameSize := 8000 * 10 / 1000 * 2
	partialFrame := make([]byte, frameSize/2) // 半帧

	// 写入半帧
	segments, err := svad.Write(partialFrame)
	if err != nil {
		t.Fatalf("写入音频失败: %v", err)
	}

	// 不应该产生片段
	if len(segments) != 0 {
		t.Error("不完整帧不应该产生片段")
	}

	// 缓冲区应该有数据
	if svad.GetBufferSize() != frameSize/2 {
		t.Errorf("缓冲区大小错误: 期望%d, 得到%d", frameSize/2, svad.GetBufferSize())
	}

	// 再写入半帧，凑成完整帧
	segments, err = svad.Write(partialFrame)
	if err != nil {
		t.Fatalf("写入音频失败: %v", err)
	}

	// 现在应该产生片段
	if len(segments) == 0 {
		t.Error("完整帧应该产生片段")
	}
}

// TestStreamVADReset 测试重置功能
func TestStreamVADReset(t *testing.T) {
	svad, err := NewStreamVAD(2, 16000, 10)
	if err != nil {
		t.Fatalf("创建StreamVAD失败: %v", err)
	}

	// 写入一些数据
	frameSize := 16000 * 10 / 1000 * 2
	audioData := make([]byte, frameSize*2)
	_, err = svad.Write(audioData)
	if err != nil {
		t.Fatalf("写入音频失败: %v", err)
	}

	// 重置
	err = svad.Reset()
	if err != nil {
		t.Fatalf("重置失败: %v", err)
	}

	// 检查状态
	if svad.GetBufferSize() != 0 {
		t.Error("重置后缓冲区应为空")
	}
	if svad.GetTotalProcessed() != 0 {
		t.Error("重置后总处理量应为0")
	}
	if len(svad.GetSegments()) != 0 {
		t.Error("重置后片段列表应为空")
	}
}

// TestStreamVADSegmentFiltering 测试片段过滤
func TestStreamVADSegmentFiltering(t *testing.T) {
	svad, err := NewStreamVAD(1, 8000, 10)
	if err != nil {
		t.Fatalf("创建StreamVAD失败: %v", err)
	}

	// 写入数据
	frameSize := 8000 * 10 / 1000 * 2
	audioData := make([]byte, frameSize*5)
	_, err = svad.Write(audioData)
	if err != nil {
		t.Fatalf("写入音频失败: %v", err)
	}

	// 获取所有片段
	allSegments := svad.GetSegments()
	if len(allSegments) == 0 {
		t.Skip("没有检测到片段")
	}

	// 过滤语音片段
	speechSegments := svad.FilterSpeechSegments()
	silenceSegments := svad.FilterSilenceSegments()

	// 总数应该匹配
	if len(speechSegments)+len(silenceSegments) != len(allSegments) {
		t.Error("过滤后的片段总数不匹配")
	}
}

// TestVoiceSegmentDuration 测试时长计算
func TestVoiceSegmentDuration(t *testing.T) {
	svad, err := NewStreamVAD(1, 16000, 20)
	if err != nil {
		t.Fatalf("创建StreamVAD失败: %v", err)
	}

	// 写入1秒音频（50帧 @ 20ms）
	frameSize := 16000 * 20 / 1000 * 2
	for i := 0; i < 50; i++ {
		audioData := make([]byte, frameSize)
		_, err = svad.Write(audioData)
		if err != nil {
			t.Fatalf("写入音频失败: %v", err)
		}
	}

	// 检查总时长
	totalDuration := svad.GetTotalDuration()
	expectedDuration := time.Second

	// 允许一点误差
	diff := totalDuration - expectedDuration
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Millisecond {
		t.Errorf("总时长错误: 期望%v, 得到%v", expectedDuration, totalDuration)
	}
}

// BenchmarkStreamVADWrite Benchmark流式写入
func BenchmarkStreamVADWrite(b *testing.B) {
	svad, _ := NewStreamVAD(1, 16000, 10)
	frameSize := 16000 * 10 / 1000 * 2
	audioData := make([]byte, frameSize)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svad.Write(audioData)
	}
}

func TestStreamVADWriteReusesBuffer(t *testing.T) {
	svad, err := NewStreamVAD(1, 16000, 10)
	if err != nil {
		t.Fatalf("创建StreamVAD失败: %v", err)
	}

	frameSize := 16000 * 10 / 1000 * 2
	audioData := make([]byte, frameSize)
	initialCap := cap(svad.buffer)

	for i := 0; i < 8; i++ {
		if _, err := svad.Write(audioData); err != nil {
			t.Fatalf("写入音频失败: %v", err)
		}
	}

	if len(svad.buffer) != 0 {
		t.Fatalf("处理完整帧后缓冲区应为空，得到 %d", len(svad.buffer))
	}

	if cap(svad.buffer) != initialCap {
		t.Fatalf("缓冲区容量不应退化: initial=%d current=%d", initialCap, cap(svad.buffer))
	}
}
