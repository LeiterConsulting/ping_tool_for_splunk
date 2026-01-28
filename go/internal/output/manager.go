package output

import (
	"context"
	"errors"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/fileout"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/hec"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/metrics"
)

type Manager struct {
	cfg           config.Config
	collectorHost string
	fileWriter    *fileout.Writer
	hecWriter     *hec.Writer
	metricsBuf    *metrics.Buffer
}

func NewManager(cfg config.Config, collectorHost string) (*Manager, error) {
	m := &Manager{cfg: cfg, collectorHost: collectorHost}

	useFile := (cfg.OutputMode == "file" || cfg.OutputMode == "both")
	useHec := (cfg.OutputMode == "hec" || cfg.OutputMode == "both") && cfg.HEC.Enabled

	if useFile {
		w, err := fileout.New(cfg.LogPath, cfg.LogRotationSizeMB)
		if err != nil {
			return nil, err
		}
		m.fileWriter = w
	}
	if useHec {
		w, err := hec.New(cfg.HEC, collectorHost)
		if err != nil {
			return nil, err
		}
		m.hecWriter = w
	}
	if cfg.Metrics.Enabled {
		m.metricsBuf = metrics.New(cfg.Metrics, collectorHost)
	}
	return m, nil
}

func (m *Manager) HandleResult(ctx context.Context, individual []models.PingEvent, summary models.SummaryEvent) error {
	emitEvents := true
	if m.cfg.Metrics.Enabled && m.cfg.Metrics.Mode == "metrics_only" {
		emitEvents = false
	}

	if emitEvents {
		if m.fileWriter != nil {
			if len(individual) > 0 {
				if err := m.fileWriter.WritePingEvents(individual); err != nil {
					return err
				}
			}
			if err := m.fileWriter.WriteOne(summary); err != nil {
				return err
			}
		}
		if m.hecWriter != nil {
			if len(individual) > 0 {
				if err := m.hecWriter.AddMany(individual); err != nil {
					return err
				}
			}
			if err := m.hecWriter.AddOne(summary); err != nil {
				return err
			}
		}
	}

	if m.metricsBuf != nil {
		if err := m.metricsBuf.AddSummary(summary); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) FlushCycle(ctx context.Context) error {
	if m.hecWriter != nil {
		if err := m.hecWriter.Flush(ctx); err != nil {
			return err
		}
	}
	if m.metricsBuf != nil {
		if err := m.metricsBuf.Flush(ctx); err != nil {
			return err
		}
	}
	if m.fileWriter != nil {
		return m.fileWriter.Flush()
	}
	return nil
}

func (m *Manager) Close() {
	if m.fileWriter != nil {
		_ = m.fileWriter.Close()
	}
	if m.hecWriter != nil {
		_ = m.hecWriter.Close()
	}
	if m.metricsBuf != nil {
		_ = m.metricsBuf.Close()
	}
}

var _ = errors.Is
