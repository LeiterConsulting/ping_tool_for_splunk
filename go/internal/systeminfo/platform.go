package systeminfo

type platformCounters struct {
	processCPU100ns   uint64
	systemIdle100ns   uint64
	systemKernel100ns uint64
	systemUser100ns   uint64
	validProcessCPU   bool
	validSystemCPU    bool
}

func uint64Pointer(value uint64) *uint64 {
	return &value
}

func float64Pointer(value float64) *float64 {
	return &value
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
