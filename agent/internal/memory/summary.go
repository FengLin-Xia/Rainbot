package memory

// SummaryMemory is a placeholder for the long-term summarization layer.
// In v0.1 this is a no-op; the interface is preserved for future integration.
type SummaryMemory struct {
	summary string
}

func NewSummaryMemory() *SummaryMemory {
	return &SummaryMemory{}
}

func (s *SummaryMemory) Get() string {
	return s.summary
}

func (s *SummaryMemory) Set(summary string) {
	s.summary = summary
}
