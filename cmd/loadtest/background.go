package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/loadtest/seeder"
	"github.com/rmorlok/authproxy/internal/service"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/spf13/cobra"
)

const (
	loadtestScenarioRefreshSweep        = "refresh-sweep"
	loadtestScenarioSchedulerSync       = "scheduler-sync"
	loadtestScenarioResourceSnapshot    = "resource-snapshot"
	loadtestScenarioStaleSetupCleanup   = "stale-setup-cleanup"
	loadtestScenarioProbeOutcomeCleanup = "probe-outcome-cleanup"

	taskTypeRefreshExpiringOAuthTokens = "oauth2:refresh_expiring_oauth_tokens"
	taskTypeResourceSnapshot           = "app_metrics:resource_snapshot"
	taskTypeCleanupStaleConnections    = "database:cleanup_stale_connections"
	taskTypeProbeOutcomeCleanup        = "core:probe_outcome_cleanup"

	defaultLoadtestQueue = "default"
)

type loadtestBackgroundSummary struct {
	ProfileName             string                 `json:"profile_name,omitempty"`
	Scenario                string                 `json:"scenario"`
	Queue                   string                 `json:"queue,omitempty"`
	Percent                 *int                   `json:"percent,omitempty"`
	TaskType                string                 `json:"task_type,omitempty"`
	TaskID                  string                 `json:"task_id,omitempty"`
	TaskQueue               string                 `json:"task_queue,omitempty"`
	ExpectedExpiringTokens  int                    `json:"expected_expiring_tokens,omitempty"`
	SchedulerTaskConfigs    int                    `json:"scheduler_task_configs,omitempty"`
	SchedulerProbeTasks     int                    `json:"scheduler_probe_tasks,omitempty"`
	StartedAt               time.Time              `json:"started_at"`
	FinishedAt              time.Time              `json:"finished_at"`
	DurationSeconds         float64                `json:"duration_seconds"`
	EnqueueDurationSeconds  float64                `json:"enqueue_duration_seconds,omitempty"`
	WaitDurationSeconds     float64                `json:"wait_duration_seconds,omitempty"`
	ProcessedDelta          int                    `json:"processed_delta,omitempty"`
	FailedDelta             int                    `json:"failed_delta,omitempty"`
	ProcessedRatePerSecond  float64                `json:"processed_rate_per_second,omitempty"`
	Before                  *loadtestQueueSnapshot `json:"before,omitempty"`
	After                   *loadtestQueueSnapshot `json:"after,omitempty"`
	MaxObserved             *loadtestQueueSnapshot `json:"max_observed,omitempty"`
	MemoryBefore            loadtestMemorySnapshot `json:"memory_before"`
	MemoryAfter             loadtestMemorySnapshot `json:"memory_after"`
	MemoryDeltaAllocBytes   int64                  `json:"memory_delta_alloc_bytes"`
	MemoryDeltaSysBytes     int64                  `json:"memory_delta_sys_bytes"`
	SchedulerTaskTypeCounts map[string]int         `json:"scheduler_task_type_counts,omitempty"`
	Artifacts               map[string]string      `json:"artifacts,omitempty"`
}

type loadtestQueueSnapshot struct {
	CapturedAt       time.Time `json:"captured_at"`
	Queue            string    `json:"queue"`
	Size             int       `json:"size"`
	Pending          int       `json:"pending"`
	Active           int       `json:"active"`
	Scheduled        int       `json:"scheduled"`
	Retry            int       `json:"retry"`
	Archived         int       `json:"archived"`
	Completed        int       `json:"completed"`
	Aggregating      int       `json:"aggregating"`
	Processed        int       `json:"processed"`
	Failed           int       `json:"failed"`
	ProcessedTotal   int       `json:"processed_total"`
	FailedTotal      int       `json:"failed_total"`
	MemoryUsage      int64     `json:"memory_usage"`
	LatencySeconds   float64   `json:"latency_seconds"`
	InFlightAndRetry int       `json:"in_flight_and_retry"`
}

type loadtestMemorySnapshot struct {
	AllocBytes      uint64 `json:"alloc_bytes"`
	TotalAllocBytes uint64 `json:"total_alloc_bytes"`
	SysBytes        uint64 `json:"sys_bytes"`
	HeapAllocBytes  uint64 `json:"heap_alloc_bytes"`
	HeapSysBytes    uint64 `json:"heap_sys_bytes"`
	NumGC           uint32 `json:"num_gc"`
}

func cmdBackground() *cobra.Command {
	var profilePath string
	var runDir string
	var scenario string
	var queue string
	var percent int
	var waitForDrain bool
	var drainTimeout time.Duration
	var pollInterval time.Duration
	var taskRetention time.Duration

	cmd := &cobra.Command{
		Use:   "background",
		Short: "Run AuthProxy load-test background job scenarios",
		RunE: func(cmd *cobra.Command, args []string) error {
			if scenario == "" {
				return fmt.Errorf("--scenario is required")
			}
			if queue == "" {
				queue = defaultLoadtestQueue
			}

			var profile seeder.Profile
			if profilePath != "" {
				loaded, err := seeder.LoadProfile(profilePath)
				if err != nil {
					return fmt.Errorf("failed to load profile: %w", err)
				}
				profile = loaded
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			dm := service.NewDependencyManager("loadtest-background", cfg)
			defer dm.ShutdownTelemetry()
			defer dm.ShutdownDatabase()

			enc := dm.GetEncryptService()
			defer enc.Shutdown()
			if err := enc.SyncKeysFromDbToMemory(ctx); err != nil {
				return fmt.Errorf("failed to sync encryption keys: %w", err)
			}

			runner := loadtestBackgroundRunner{
				dm:            dm,
				profile:       profile,
				runDir:        runDir,
				queue:         queue,
				percent:       percent,
				waitForDrain:  waitForDrain,
				drainTimeout:  drainTimeout,
				pollInterval:  pollInterval,
				taskRetention: taskRetention,
			}

			summary, err := runner.run(ctx, scenario)
			if err != nil {
				return err
			}

			if err := runner.writeSummary(summary); err != nil {
				return err
			}

			encoded, err := json.MarshalIndent(summary, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(encoded))
			return nil
		},
	}

	cmd.Flags().StringVar(&profilePath, "profile", "", "load-test profile YAML")
	cmd.Flags().StringVar(&runDir, "run-dir", "", "artifact directory to populate")
	cmd.Flags().StringVar(&scenario, "scenario", "", "scenario: refresh-sweep, scheduler-sync, resource-snapshot, stale-setup-cleanup, probe-outcome-cleanup")
	cmd.Flags().StringVar(&queue, "queue", defaultLoadtestQueue, "Asynq queue to inspect")
	cmd.Flags().IntVar(&percent, "percent", 0, "profile percentage currently under test; used for artifact metadata")
	cmd.Flags().BoolVar(&waitForDrain, "wait-drain", true, "wait for the queue to return to its baseline pending/active/retry counts")
	cmd.Flags().DurationVar(&drainTimeout, "drain-timeout", 30*time.Minute, "maximum time to wait for worker drain")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 5*time.Second, "queue polling interval while waiting for drain")
	cmd.Flags().DurationVar(&taskRetention, "task-retention", 24*time.Hour, "retention to apply to load-test trigger tasks")
	return cmd
}

type loadtestBackgroundRunner struct {
	dm            *service.DependencyManager
	profile       seeder.Profile
	runDir        string
	queue         string
	percent       int
	waitForDrain  bool
	drainTimeout  time.Duration
	pollInterval  time.Duration
	taskRetention time.Duration
	queueSamples  []loadtestQueueSnapshot
}

func (r *loadtestBackgroundRunner) run(ctx context.Context, scenario string) (*loadtestBackgroundSummary, error) {
	startedAt := time.Now().UTC()
	memBefore := readLoadtestMemory()
	summary := &loadtestBackgroundSummary{
		ProfileName:  r.profile.Name,
		Scenario:     scenario,
		Queue:        r.queue,
		StartedAt:    startedAt,
		MemoryBefore: memBefore,
		Artifacts:    map[string]string{},
	}
	if r.percent > 0 {
		pct := r.percent
		summary.Percent = &pct
	}

	var err error
	switch scenario {
	case loadtestScenarioRefreshSweep:
		summary.ExpectedExpiringTokens, err = r.countExpiringOAuthTokens(ctx)
		if err == nil {
			err = r.runEnqueueScenario(ctx, summary, taskTypeRefreshExpiringOAuthTokens, nil)
		}
	case loadtestScenarioResourceSnapshot:
		err = r.runEnqueueScenario(ctx, summary, taskTypeResourceSnapshot, nil)
	case loadtestScenarioStaleSetupCleanup:
		err = r.runEnqueueScenario(ctx, summary, taskTypeCleanupStaleConnections, nil)
	case loadtestScenarioProbeOutcomeCleanup:
		err = r.runEnqueueScenario(ctx, summary, taskTypeProbeOutcomeCleanup, []byte(`{"retention_seconds":0}`))
	case loadtestScenarioSchedulerSync:
		err = r.runSchedulerSync(ctx, summary)
	default:
		err = fmt.Errorf("unknown background load-test scenario: %s", scenario)
	}
	if err != nil {
		return nil, err
	}

	finishedAt := time.Now().UTC()
	memAfter := readLoadtestMemory()
	summary.FinishedAt = finishedAt
	summary.DurationSeconds = finishedAt.Sub(startedAt).Seconds()
	summary.MemoryAfter = memAfter
	summary.MemoryDeltaAllocBytes = int64(memAfter.AllocBytes) - int64(memBefore.AllocBytes)
	summary.MemoryDeltaSysBytes = int64(memAfter.SysBytes) - int64(memBefore.SysBytes)
	return summary, nil
}

func (r *loadtestBackgroundRunner) runEnqueueScenario(ctx context.Context, summary *loadtestBackgroundSummary, taskType string, payload []byte) error {
	inspector := r.dm.GetAsyncInspector()
	before, err := r.queueSnapshot(inspector, summary.Queue)
	if err != nil {
		return fmt.Errorf("capture queue before scenario: %w", err)
	}
	summary.Before = before
	r.queueSamples = append(r.queueSamples, *before)

	task := asynq.NewTask(taskType, payload)
	enqueueOpts := []asynq.Option{asynq.Retention(r.taskRetention)}
	if r.queue != "" {
		enqueueOpts = append(enqueueOpts, asynq.Queue(r.queue))
	}
	enqueueStart := time.Now()
	info, err := r.dm.GetAsyncClient().EnqueueContext(ctx, task, enqueueOpts...)
	if err != nil {
		return fmt.Errorf("enqueue %s: %w", taskType, err)
	}
	summary.EnqueueDurationSeconds = time.Since(enqueueStart).Seconds()
	summary.TaskType = taskType
	summary.TaskID = info.ID
	summary.TaskQueue = info.Queue

	if r.waitForDrain {
		waitStart := time.Now()
		maxObserved, err := r.waitForQueueDrain(ctx, inspector, before)
		if err != nil {
			return err
		}
		summary.WaitDurationSeconds = time.Since(waitStart).Seconds()
		summary.MaxObserved = maxObserved
	}

	after, err := r.queueSnapshot(inspector, summary.Queue)
	if err != nil {
		return fmt.Errorf("capture queue after scenario: %w", err)
	}
	summary.After = after
	r.queueSamples = append(r.queueSamples, *after)
	summary.ProcessedDelta = after.ProcessedTotal - before.ProcessedTotal
	summary.FailedDelta = after.FailedTotal - before.FailedTotal
	if summary.WaitDurationSeconds > 0 {
		summary.ProcessedRatePerSecond = float64(summary.ProcessedDelta) / summary.WaitDurationSeconds
	}
	return nil
}

func (r *loadtestBackgroundRunner) runSchedulerSync(ctx context.Context, summary *loadtestBackgroundSummary) error {
	before, err := r.queueSnapshot(r.dm.GetAsyncInspector(), summary.Queue)
	if err == nil {
		summary.Before = before
		r.queueSamples = append(r.queueSamples, *before)
	}

	start := time.Now()
	configs := r.dm.GetCoreService().GetCronTasks()
	summary.WaitDurationSeconds = time.Since(start).Seconds()
	summary.SchedulerTaskConfigs = len(configs)
	typeCounts := make(map[string]int)
	cronspecCounts := make(map[string]map[string]int)
	for _, cfg := range configs {
		if cfg == nil || cfg.Task == nil {
			continue
		}
		taskType := cfg.Task.Type()
		typeCounts[taskType]++
		if taskType == "core:probe" {
			summary.SchedulerProbeTasks++
		}
		if _, ok := cronspecCounts[taskType]; !ok {
			cronspecCounts[taskType] = make(map[string]int)
		}
		cronspecCounts[taskType][cfg.Cronspec]++
	}
	summary.SchedulerTaskTypeCounts = typeCounts
	path, err := r.writeSchedulerTasks(cronspecCounts)
	if err != nil {
		return err
	}
	if path != "" {
		summary.Artifacts["scheduler_task_configs"] = path
	}

	after, err := r.queueSnapshot(r.dm.GetAsyncInspector(), summary.Queue)
	if err == nil {
		summary.After = after
		r.queueSamples = append(r.queueSamples, *after)
	}
	return ctx.Err()
}

func (r *loadtestBackgroundRunner) countExpiringOAuthTokens(ctx context.Context) (int, error) {
	refreshWithin := r.dm.GetConfigRoot().Oauth.GetRefreshTokensTimeBeforeExpiryOrDefault()
	total := 0
	err := r.dm.GetDatabase().EnumerateOAuth2TokensExpiringWithin(
		ctx,
		refreshWithin,
		func(tokens []*database.OAuth2TokenWithConnection, lastPage bool) (pagination.KeepGoing, error) {
			total += len(tokens)
			return pagination.Continue, nil
		},
	)
	return total, err
}

func (r *loadtestBackgroundRunner) waitForQueueDrain(ctx context.Context, inspector *asynq.Inspector, baseline *loadtestQueueSnapshot) (*loadtestQueueSnapshot, error) {
	deadline := time.Now().Add(r.drainTimeout)
	maxObserved := *baseline
	poll := r.pollInterval
	if poll <= 0 {
		poll = 5 * time.Second
	}
	for {
		current, err := r.queueSnapshot(inspector, baseline.Queue)
		if err != nil {
			return nil, fmt.Errorf("capture queue while waiting for drain: %w", err)
		}
		r.queueSamples = append(r.queueSamples, *current)
		maxObserved = maxQueueSnapshot(maxObserved, *current)

		if queueAtOrBelowBaseline(current, baseline) && queueProgressed(current, baseline) {
			return &maxObserved, nil
		}
		if time.Now().After(deadline) {
			return &maxObserved, fmt.Errorf(
				"queue %s did not drain within %s: pending=%d active=%d retry=%d baseline_pending=%d baseline_active=%d baseline_retry=%d",
				baseline.Queue,
				r.drainTimeout,
				current.Pending,
				current.Active,
				current.Retry,
				baseline.Pending,
				baseline.Active,
				baseline.Retry,
			)
		}

		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return &maxObserved, ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *loadtestBackgroundRunner) queueSnapshot(inspector *asynq.Inspector, queue string) (*loadtestQueueSnapshot, error) {
	qi, err := inspector.GetQueueInfo(queue)
	if err != nil {
		return nil, err
	}
	return &loadtestQueueSnapshot{
		CapturedAt:       time.Now().UTC(),
		Queue:            qi.Queue,
		Size:             qi.Size,
		Pending:          qi.Pending,
		Active:           qi.Active,
		Scheduled:        qi.Scheduled,
		Retry:            qi.Retry,
		Archived:         qi.Archived,
		Completed:        qi.Completed,
		Aggregating:      qi.Aggregating,
		Processed:        qi.Processed,
		Failed:           qi.Failed,
		ProcessedTotal:   qi.ProcessedTotal,
		FailedTotal:      qi.FailedTotal,
		MemoryUsage:      qi.MemoryUsage,
		LatencySeconds:   qi.Latency.Seconds(),
		InFlightAndRetry: qi.Pending + qi.Active + qi.Retry,
	}, nil
}

func queueAtOrBelowBaseline(current, baseline *loadtestQueueSnapshot) bool {
	return current.Pending <= baseline.Pending &&
		current.Active <= baseline.Active &&
		current.Retry <= baseline.Retry
}

func queueProgressed(current, baseline *loadtestQueueSnapshot) bool {
	return current.ProcessedTotal > baseline.ProcessedTotal ||
		current.FailedTotal > baseline.FailedTotal
}

func maxQueueSnapshot(a, b loadtestQueueSnapshot) loadtestQueueSnapshot {
	result := a
	if b.Size > result.Size {
		result.Size = b.Size
	}
	if b.Pending > result.Pending {
		result.Pending = b.Pending
	}
	if b.Active > result.Active {
		result.Active = b.Active
	}
	if b.Scheduled > result.Scheduled {
		result.Scheduled = b.Scheduled
	}
	if b.Retry > result.Retry {
		result.Retry = b.Retry
	}
	if b.MemoryUsage > result.MemoryUsage {
		result.MemoryUsage = b.MemoryUsage
	}
	if b.InFlightAndRetry > result.InFlightAndRetry {
		result.InFlightAndRetry = b.InFlightAndRetry
	}
	result.CapturedAt = b.CapturedAt
	return result
}

func readLoadtestMemory() loadtestMemorySnapshot {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return loadtestMemorySnapshot{
		AllocBytes:      stats.Alloc,
		TotalAllocBytes: stats.TotalAlloc,
		SysBytes:        stats.Sys,
		HeapAllocBytes:  stats.HeapAlloc,
		HeapSysBytes:    stats.HeapSys,
		NumGC:           stats.NumGC,
	}
}

func (r *loadtestBackgroundRunner) writeSummary(summary *loadtestBackgroundSummary) error {
	if r.runDir == "" {
		return nil
	}
	if err := os.MkdirAll(r.runDir, 0o755); err != nil {
		return err
	}
	if len(r.queueSamples) > 0 {
		path := filepath.Join(r.runDir, "queue-samples.tsv")
		if err := r.writeQueueSamples(path); err != nil {
			return err
		}
		summary.Artifacts["queue_samples"] = path
	}
	path := filepath.Join(r.runDir, "background-summary.json")
	summary.Artifacts["summary"] = path
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	return nil
}

func (r *loadtestBackgroundRunner) writeQueueSamples(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintln(file, "captured_at\tqueue\tsize\tpending\tactive\tscheduled\tretry\tarchived\tcompleted\taggregating\tprocessed\tfailed\tprocessed_total\tfailed_total\tmemory_usage\tlatency_seconds\tin_flight_and_retry")
	for _, s := range r.queueSamples {
		fmt.Fprintf(file, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%.6f\t%d\n",
			s.CapturedAt.Format(time.RFC3339Nano),
			s.Queue,
			s.Size,
			s.Pending,
			s.Active,
			s.Scheduled,
			s.Retry,
			s.Archived,
			s.Completed,
			s.Aggregating,
			s.Processed,
			s.Failed,
			s.ProcessedTotal,
			s.FailedTotal,
			s.MemoryUsage,
			s.LatencySeconds,
			s.InFlightAndRetry,
		)
	}
	return nil
}

func (r *loadtestBackgroundRunner) writeSchedulerTasks(counts map[string]map[string]int) (string, error) {
	if r.runDir == "" {
		return "", nil
	}
	if err := os.MkdirAll(r.runDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(r.runDir, "scheduler-task-configs.tsv")
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	fmt.Fprintln(file, "task_type\tcronspec\tcount")

	taskTypes := make([]string, 0, len(counts))
	for taskType := range counts {
		taskTypes = append(taskTypes, taskType)
	}
	sort.Strings(taskTypes)
	for _, taskType := range taskTypes {
		cronspecs := make([]string, 0, len(counts[taskType]))
		for cronspec := range counts[taskType] {
			cronspecs = append(cronspecs, cronspec)
		}
		sort.Strings(cronspecs)
		for _, cronspec := range cronspecs {
			fmt.Fprintf(file, "%s\t%s\t%d\n", taskType, cronspec, counts[taskType][cronspec])
		}
	}
	return path, nil
}
