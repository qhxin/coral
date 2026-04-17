package main

import (
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// MemoryEntry 单个记忆条目
type MemoryEntry struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	Importance float64   `json:"importance"` // 0-1
	Tags       []string  `json:"tags"`
	Embedding  []float32 `json:"-"` // 内存中的特征向量，不持久化
}

// RAGMemory 基于语义的检索增强记忆系统
type RAGMemory struct {
	FS       *WorkspaceFS
	entries  []MemoryEntry
	features map[string][]float32 // ID -> 特征向量缓存
}

// NewRAGMemory 创建RAG记忆系统
func NewRAGMemory(fs *WorkspaceFS) *RAGMemory {
	return &RAGMemory{
		FS:       fs,
		features: make(map[string][]float32),
	}
}

// Load 从MEMORY.md加载所有记忆条目
func (m *RAGMemory) Load() error {
	content, err := m.FS.Read("MEMORY.md")
	if err != nil {
		// 文件不存在时返回空列表
		m.entries = []MemoryEntry{}
		return nil
	}

	m.entries = parseMemoryEntries(content)

	// 预计算所有条目的特征向量
	for i := range m.entries {
		m.entries[i].Embedding = m.bm25Features(m.entries[i].Content)
		m.features[m.entries[i].ID] = m.entries[i].Embedding
	}

	return nil
}

// Retrieve 检索与查询最相关的topK条记忆
func (m *RAGMemory) Retrieve(query string, topK int) []MemoryEntry {
	if len(m.entries) == 0 {
		return nil
	}

	queryVec := m.bm25Features(query)

	// 计算相似度并排序
	type scoredEntry struct {
		entry MemoryEntry
		score float64
	}

	scored := make([]scoredEntry, len(m.entries))
	for i, e := range m.entries {
		var score float64
		if e.Embedding != nil {
			score = cosineSimilarity(queryVec, e.Embedding)
		} else {
			// 如果embedding未计算，实时计算
			entryVec := m.bm25Features(e.Content)
			score = cosineSimilarity(queryVec, entryVec)
		}
		scored[i] = scoredEntry{entry: e, score: score}
	}

	// 按相似度降序排序
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// 返回topK，过滤低相似度的（<0.3）
	result := make([]MemoryEntry, 0, topK)
	for i := 0; i < len(scored) && len(result) < topK; i++ {
		if scored[i].score > 0.3 {
			result = append(result, scored[i].entry)
		}
	}

	return result
}

// bm25Features 提取BM25特征向量（零外部依赖）
func (m *RAGMemory) bm25Features(text string) []float32 {
	terms := tokenizeForRAG(text)

	// 使用1000维的稀疏向量（哈希桶）
	features := make([]float32, 1000)

	// 词频统计
	termFreq := make(map[string]int)
	for _, term := range terms {
		termFreq[term]++
	}

	// 计算TF-IDF风格的权重，哈希到固定维度
	for term, freq := range termFreq {
		// 简单TF（可改进为BM25公式）
		tf := float32(math.Log1p(float64(freq)))

		// 哈希到桶
		hash := sha256.Sum256([]byte(term))
		bucket := int(hash[0]) % 1000

		features[bucket] += tf
	}

	// L2归一化
	return normalizeVector(features)
}

// tokenizeForRAG 简单分词（BM25专用）
func tokenizeForRAG(text string) []string {
	// 转小写，移除非字母数字
	text = strings.ToLower(text)
	var tokens []string

	// 按非字母数字字符分割
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r < 128
	})

	// 过滤停用词和短词
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"的": true, "是": true, "在": true, "和": true, "了": true,
	}

	for _, f := range fields {
		if len(f) > 2 && !stopWords[f] {
			tokens = append(tokens, f)
		}
	}

	return tokens
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct float64
	var normA float64
	var normB float64

	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// normalizeVector L2归一化
func normalizeVector(v []float32) []float32 {
	var norm float64
	for _, x := range v {
		norm += float64(x * x)
	}

	if norm == 0 {
		return v
	}

	norm = math.Sqrt(norm)
	result := make([]float32, len(v))
	for i, x := range v {
		result[i] = float32(float64(x) / norm)
	}

	return result
}

// parseMemoryEntries 解析MEMORY.md内容为条目列表
func parseMemoryEntries(content string) []MemoryEntry {
	var entries []MemoryEntry

	// 按 "## Memo at" 分割
	parts := strings.Split(content, "## Memo at")

	for _, part := range parts[1:] { // 跳过第一部分（通常是头部说明）
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 解析时间戳和内容
		lines := strings.SplitN(part, "\n", 2)
		if len(lines) < 2 {
			continue
		}

		timestampStr := strings.TrimSpace(lines[0])
		content := strings.TrimSpace(lines[1])

		ts, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			ts = Now()
		}

		// 生成ID
		hash := sha256.Sum256([]byte(content))
		id := fmt.Sprintf("%x", hash[:8])

		entries = append(entries, MemoryEntry{
			ID:         id,
			Content:    content,
			Timestamp:  ts,
			Importance: 0.5, // 默认重要性
		})
	}

	return entries
}

// AddEntry 添加新记忆条目
func (m *RAGMemory) AddEntry(content string) error {
	// 生成ID
	hash := sha256.Sum256([]byte(content))
	id := fmt.Sprintf("%x", hash[:8])

	entry := MemoryEntry{
		ID:         id,
		Content:    content,
		Timestamp:  Now(),
		Importance: 0.5,
		Embedding:  m.bm25Features(content),
	}

	m.entries = append(m.entries, entry)
	m.features[id] = entry.Embedding

	return nil
}
