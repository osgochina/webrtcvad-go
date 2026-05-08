package webrtcvad

import (
	"io"
	"os"
	"testing"
)

func testBytesToInt16(buf []byte) []int16 {
	samples := make([]int16, len(buf)/2)
	for i := range samples {
		samples[i] = int16(buf[i*2]) | (int16(buf[i*2+1]) << 8)
	}
	return samples
}

// TestConstructor 测试VAD实例创建
func TestConstructor(t *testing.T) {
	vad, err := New(0)
	if err != nil {
		t.Fatalf("Failed to create VAD: %v", err)
	}
	if vad == nil {
		t.Fatal("VAD instance is nil")
	}
}

// TestSetMode 测试设置模式
func TestSetMode(t *testing.T) {
	vad, err := New(0)
	if err != nil {
		t.Fatalf("Failed to create VAD: %v", err)
	}

	// 测试有效模式
	for mode := 0; mode <= 3; mode++ {
		err := vad.SetMode(mode)
		if err != nil {
			t.Errorf("Failed to set mode %d: %v", mode, err)
		}
	}

	// 测试无效模式
	err = vad.SetMode(4)
	if err == nil {
		t.Error("Expected error for invalid mode 4, got nil")
	}

	err = vad.SetMode(-1)
	if err == nil {
		t.Error("Expected error for invalid mode -1, got nil")
	}
}

// TestValidRateAndFrameLength 测试采样率和帧长度验证
func TestValidRateAndFrameLength(t *testing.T) {
	tests := []struct {
		rate        int
		frameLength int
		expected    bool
	}{
		{8000, 80, true},    // 10ms @ 8kHz
		{8000, 160, true},   // 20ms @ 8kHz
		{8000, 240, true},   // 30ms @ 8kHz
		{16000, 160, true},  // 10ms @ 16kHz
		{16000, 320, true},  // 20ms @ 16kHz
		{16000, 480, true},  // 30ms @ 16kHz
		{32000, 320, true},  // 10ms @ 32kHz
		{32000, 640, true},  // 20ms @ 32kHz
		{32000, 960, true},  // 30ms @ 32kHz
		{48000, 480, true},  // 10ms @ 48kHz
		{48000, 960, true},  // 20ms @ 48kHz
		{48000, 1440, true}, // 30ms @ 48kHz
		{32000, 160, false}, // 无效组合
		{8000, 100, false},  // 无效帧长度
		{16000, 100, false}, // 无效帧长度
		{44100, 441, false}, // 无效采样率
	}

	for _, tt := range tests {
		result := ValidRateAndFrameLength(tt.rate, tt.frameLength)
		if result != tt.expected {
			t.Errorf("ValidRateAndFrameLength(%d, %d) = %v, expected %v",
				tt.rate, tt.frameLength, result, tt.expected)
		}
	}
}

// TestProcessZeroes 测试处理全零音频（应该检测为非语音）
func TestProcessZeroes(t *testing.T) {
	frameLen := 160
	sampleRate := 16000

	if !ValidRateAndFrameLength(sampleRate, frameLen) {
		t.Fatalf("Invalid rate and frame length: %d, %d", sampleRate, frameLen)
	}

	// 创建全零样本（静音）
	sample := make([]byte, frameLen*2)

	vad, err := New(0)
	if err != nil {
		t.Fatalf("Failed to create VAD: %v", err)
	}

	isSpeech, err := vad.IsSpeech(sample, sampleRate)
	if err != nil {
		t.Fatalf("Failed to process audio: %v", err)
	}

	if isSpeech {
		t.Error("Expected silence (false), but got speech (true)")
	}
}

// TestProcessFile 测试处理实际音频文件
func TestProcessFile(t *testing.T) {
	// 尝试读取测试音频文件
	data, err := os.ReadFile("./test/test-audio.raw")
	if err != nil {
		t.Skip("Test audio file not found, skipping test")
		return
	}

	frameMs := 30
	sampleRate := 8000
	bytesPerSample := 2
	n := int(sampleRate * bytesPerSample * frameMs / 1000)
	frameLen := n / 2

	if !ValidRateAndFrameLength(sampleRate, frameLen) {
		t.Fatalf("Invalid rate and frame length: %d, %d", sampleRate, frameLen)
	}

	// 将数据分割成帧
	var chunks [][]byte
	for pos := 0; pos+n <= len(data); pos += n {
		chunk := make([]byte, n)
		copy(chunk, data[pos:pos+n])
		chunks = append(chunks, chunk)
	}

	// 不同模式下的预期结果（根据Python测试）
	expecteds := []string{
		"011110111111111111111111111100",
		"011110111111111111111111111100",
		"000000111111111111111111110000",
		"000000111111111111111100000000",
	}

	for mode := 0; mode <= 3; mode++ {
		vad, err := New(mode)
		if err != nil {
			t.Fatalf("Failed to create VAD with mode %d: %v", mode, err)
		}

		var result string
		for _, chunk := range chunks {
			voiced, err := vad.IsSpeech(chunk, sampleRate)
			if err != nil {
				t.Fatalf("Failed to process chunk in mode %d: %v", mode, err)
			}
			if voiced {
				result += "1"
			} else {
				result += "0"
			}
		}

		if result != expecteds[mode] {
			t.Errorf("Mode %d: expected %s, got %s", mode, expecteds[mode], result)
		} else {
			t.Logf("Mode %d: PASS - %s", mode, result)
		}
	}
}

// BenchmarkIsSpeech 基准测试
func BenchmarkIsSpeech(b *testing.B) {
	frameLen := 160
	sampleRate := 16000
	sample := make([]byte, frameLen*2)

	// 填充一些示例数据
	for i := range sample {
		sample[i] = byte(i % 256)
	}

	vad, err := New(1)
	if err != nil {
		b.Fatalf("Failed to create VAD: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := vad.IsSpeech(sample, sampleRate)
		if err != nil {
			b.Fatalf("Failed to process audio: %v", err)
		}
	}
}

func TestIsSpeechInt16MatchesBytes(t *testing.T) {
	frameLen := 160
	sampleRate := 16000
	sample := make([]byte, frameLen*2)

	for i := range sample {
		sample[i] = byte(i % 256)
	}

	samples := testBytesToInt16(sample)

	vadBytes, err := New(1)
	if err != nil {
		t.Fatalf("Failed to create byte VAD: %v", err)
	}

	vadInt16, err := New(1)
	if err != nil {
		t.Fatalf("Failed to create int16 VAD: %v", err)
	}

	gotBytes, err := vadBytes.IsSpeech(sample, sampleRate)
	if err != nil {
		t.Fatalf("Failed to process bytes: %v", err)
	}

	gotInt16, err := vadInt16.IsSpeechInt16(samples, sampleRate)
	if err != nil {
		t.Fatalf("Failed to process int16 samples: %v", err)
	}

	if gotBytes != gotInt16 {
		t.Fatalf("IsSpeech mismatch: bytes=%v int16=%v", gotBytes, gotInt16)
	}
}

// BenchmarkIsSpeech8kHz 8kHz采样率基准测试
func BenchmarkIsSpeech8kHz(b *testing.B) {
	frameLen := 80
	sampleRate := 8000
	sample := make([]byte, frameLen*2)

	for i := range sample {
		sample[i] = byte(i % 256)
	}

	vad, err := New(1)
	if err != nil {
		b.Fatalf("Failed to create VAD: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := vad.IsSpeech(sample, sampleRate)
		if err != nil {
			b.Fatalf("Failed to process audio: %v", err)
		}
	}
}

// BenchmarkIsSpeech48kHz 48kHz采样率基准测试
func BenchmarkIsSpeech48kHz(b *testing.B) {
	frameLen := 480
	sampleRate := 48000
	sample := make([]byte, frameLen*2)

	for i := range sample {
		sample[i] = byte(i % 256)
	}

	vad, err := New(1)
	if err != nil {
		b.Fatalf("Failed to create VAD: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := vad.IsSpeech(sample, sampleRate)
		if err != nil {
			b.Fatalf("Failed to process audio: %v", err)
		}
	}
}

func BenchmarkIsSpeechInt16(b *testing.B) {
	frameLen := 160
	sampleRate := 16000
	sample := make([]byte, frameLen*2)

	for i := range sample {
		sample[i] = byte(i % 256)
	}

	samples := testBytesToInt16(sample)

	vad, err := New(1)
	if err != nil {
		b.Fatalf("Failed to create VAD: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := vad.IsSpeechInt16(samples, sampleRate)
		if err != nil {
			b.Fatalf("Failed to process audio: %v", err)
		}
	}
}

// TestPCM 测试处理test目录中的PCM音频文件
func TestPCM(t *testing.T) {
	const (
		// VadMode VAD模式
		VadMode = 0
		// SampleRate 采样率
		SampleRate = 16000
		// BitDepth 位深度
		BitDepth = 16
		// FrameDuration 帧时长(ms)
		FrameDuration = 20
	)

	var (
		frameIndex  = 0
		frameBuffer = make([]byte, SampleRate/1000*FrameDuration*BitDepth/8)
	)

	// 打开测试音频文件
	audioFile, err := os.Open("./test/test.pcm")
	if err != nil {
		t.Skipf("Test audio file not found: %v", err)
		return
	}
	defer audioFile.Close()

	// 创建VAD实例
	vad, err := New(VadMode)
	if err != nil {
		t.Fatalf("Failed to create VAD: %v", err)
	}

	// 逐帧读取并处理音频
	for {
		n, err := audioFile.Read(frameBuffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read audio frame: %v", err)
		}

		// 确保读取了完整的帧
		if n != len(frameBuffer) {
			t.Logf("Incomplete frame at end of file, skipping")
			break
		}

		// 检测语音活动
		isSpeech, err := vad.IsSpeech(frameBuffer, SampleRate)
		if err != nil {
			t.Fatalf("Failed to process frame %d: %v", frameIndex, err)
		}

		t.Logf("Frame: %d, Active: %v", frameIndex, isSpeech)
		frameIndex++
	}

	if frameIndex == 0 {
		t.Error("No frames were processed")
	}
}
