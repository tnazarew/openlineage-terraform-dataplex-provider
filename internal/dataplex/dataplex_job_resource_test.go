package dataplex

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OpenLineage/openlineage/byool/terraform/openlineage-base-resource/ol"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ── fakeLineageAPI ────────────────────────────────────────────────────────────
//
// fakeLineageAPI is a manual test double for the lineageAPI interface.
// Each method records its calls and returns pre-configured values,
// avoiding any real gRPC connection.

type fakeLineageAPI struct {
	// emitAndCapture
	emitResult *emitResult
	emitErr    error
	emitCalls  int

	// getProcess
	processResult *processInfo
	processErr    error
	processCalls  []string

	// getLatestRun
	runResult *runInfo
	runErr    error
	runCalls  []string

	// deleteProcess
	deleteErr   error
	deleteCalls []string

	// searchProcess
	searchResult *processInfo
	searchErr    error
	searchCalls  []struct{ namespace, jobName string }
}

func (f *fakeLineageAPI) emitAndCapture(_ context.Context, _ any) (*emitResult, error) {
	f.emitCalls++
	return f.emitResult, f.emitErr
}
func (f *fakeLineageAPI) getProcess(_ context.Context, name string) (*processInfo, error) {
	f.processCalls = append(f.processCalls, name)
	return f.processResult, f.processErr
}
func (f *fakeLineageAPI) getLatestRun(_ context.Context, processName string) (*runInfo, error) {
	f.runCalls = append(f.runCalls, processName)
	return f.runResult, f.runErr
}
func (f *fakeLineageAPI) deleteProcess(_ context.Context, name string) error {
	f.deleteCalls = append(f.deleteCalls, name)
	return f.deleteErr
}
func (f *fakeLineageAPI) searchProcess(_ context.Context, ns, job string) (*processInfo, error) {
	f.searchCalls = append(f.searchCalls, struct{ namespace, jobName string }{ns, job})
	return f.searchResult, f.searchErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestResource(api lineageAPI) *DataplexJobResource {
	r := &DataplexJobResource{dpClient: api}
	r.Backend = r
	return r
}

func modelWithProcessName(name string) *DataplexJobModel {
	m := &DataplexJobModel{}
	m.ProcessName = types.StringValue(name)
	return m
}

// ── Capability ────────────────────────────────────────────────────────────────

func TestCapability_EnablesExpectedJobFacets(t *testing.T) {
	cap := (&DataplexJobResource{}).Capability()

	enabled := []ol.JobFacet{ol.FacetJobType, ol.FacetJobOwnership}
	for _, f := range enabled {
		if !cap.IsEnabled(f) {
			t.Errorf("Capability: expected job facet %v to be enabled", f)
		}
	}

	disabled := []ol.JobFacet{ol.FacetJobSQL, ol.FacetJobDocumentation, ol.FacetJobSourceCode, ol.FacetJobTags}
	for _, f := range disabled {
		if cap.IsEnabled(f) {
			t.Errorf("Capability: expected job facet %v to be disabled", f)
		}
	}
}

func TestCapability_EnablesExpectedDatasetFacets(t *testing.T) {
	cap := (&DataplexJobResource{}).Capability()

	enabled := []ol.DatasetFacet{ol.FacetDatasetSymlinks, ol.FacetDatasetCatalog, ol.FacetDatasetColumnLineage}
	for _, f := range enabled {
		if !cap.IsDatasetEnabled(f) {
			t.Errorf("Capability: expected dataset facet %v to be enabled", f)
		}
	}

	disabled := []ol.DatasetFacet{
		ol.FacetDatasetSchema, ol.FacetDatasetStorage, ol.FacetDatasetOwnership,
		ol.FacetDatasetVersion, ol.FacetDatasetDataSource, ol.FacetDatasetTags,
	}
	for _, f := range disabled {
		if cap.IsDatasetEnabled(f) {
			t.Errorf("Capability: expected dataset facet %v to be disabled", f)
		}
	}
}

func TestConsumerAttributes_ContainsExpectedKeys(t *testing.T) {
	attrs := (&DataplexJobResource{}).ConsumerAttributes()

	for _, key := range []string{"process_name", "run_name", "lineage_event_name", "update_time"} {
		if _, ok := attrs[key]; !ok {
			t.Errorf("ConsumerAttributes: expected key %q", key)
		}
	}
}

func TestConsumerBlocks_IsEmpty(t *testing.T) {
	if n := len((&DataplexJobResource{}).ConsumerBlocks()); n != 0 {
		t.Errorf("ConsumerBlocks: expected empty map, got %d entries", n)
	}
}

func TestNewModel_ReturnsDataplexJobModel(t *testing.T) {
	if _, ok := (&DataplexJobResource{}).NewModel().(*DataplexJobModel); !ok {
		t.Error("NewModel: expected *DataplexJobModel")
	}
}

// ── ConsumerEmit ──────────────────────────────────────────────────────────────

func TestConsumerEmit_SetsProcessName(t *testing.T) {
	fake := &fakeLineageAPI{
		emitResult: &emitResult{ProcessName: "projects/p/locations/l/processes/abc"},
	}
	model := &DataplexJobModel{}

	newTestResource(fake).ConsumerEmit(context.Background(), model)

	if got := model.ProcessName.ValueString(); got != "projects/p/locations/l/processes/abc" {
		t.Errorf("expected ProcessName = %q, got %q", "projects/p/locations/l/processes/abc", got)
	}
}

func TestConsumerEmit_SetsRunName(t *testing.T) {
	fake := &fakeLineageAPI{
		emitResult: &emitResult{RunName: "projects/p/locations/l/processes/abc/runs/xyz"},
	}
	model := &DataplexJobModel{}

	newTestResource(fake).ConsumerEmit(context.Background(), model)

	if got := model.RunName.ValueString(); got != "projects/p/locations/l/processes/abc/runs/xyz" {
		t.Errorf("expected RunName = %q, got %q", "projects/p/locations/l/processes/abc/runs/xyz", got)
	}
}

func TestConsumerEmit_SetsLineageEventNameFromFirstEvent(t *testing.T) {
	fake := &fakeLineageAPI{
		emitResult: &emitResult{LineageEventNames: []string{"event-1", "event-2"}},
	}
	model := &DataplexJobModel{}

	newTestResource(fake).ConsumerEmit(context.Background(), model)

	if got := model.LineageEventName.ValueString(); got != "event-1" {
		t.Errorf("expected first event name %q, got %q", "event-1", got)
	}
}

func TestConsumerEmit_LineageEventNameEmptyWhenNoEvents(t *testing.T) {
	fake := &fakeLineageAPI{emitResult: &emitResult{LineageEventNames: []string{}}}
	model := &DataplexJobModel{}

	newTestResource(fake).ConsumerEmit(context.Background(), model)

	if got := model.LineageEventName.ValueString(); got != "" {
		t.Errorf("expected empty LineageEventName, got %q", got)
	}
}

func TestConsumerEmit_CallsEmitExactlyOnce(t *testing.T) {
	fake := &fakeLineageAPI{emitResult: &emitResult{}}

	newTestResource(fake).ConsumerEmit(context.Background(), &DataplexJobModel{})

	if fake.emitCalls != 1 {
		t.Errorf("expected emitAndCapture called once, got %d", fake.emitCalls)
	}
}

func TestConsumerEmit_CallsRefreshRunStateWithNewProcessName(t *testing.T) {
	fake := &fakeLineageAPI{
		emitResult: &emitResult{ProcessName: "projects/p/processes/abc"},
	}

	newTestResource(fake).ConsumerEmit(context.Background(), &DataplexJobModel{})

	if len(fake.runCalls) == 0 {
		t.Fatal("expected getLatestRun to be called after emit (refreshRunState)")
	}
	if fake.runCalls[0] != "projects/p/processes/abc" {
		t.Errorf("expected getLatestRun(%q), got %q", "projects/p/processes/abc", fake.runCalls[0])
	}
}

func TestConsumerEmit_SetsUpdateTimeFromEndTime(t *testing.T) {
	endTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	fake := &fakeLineageAPI{
		emitResult: &emitResult{ProcessName: "projects/p/processes/abc"},
		runResult:  &runInfo{EndTime: endTime},
	}
	model := &DataplexJobModel{}

	newTestResource(fake).ConsumerEmit(context.Background(), model)

	if got := model.UpdateTime.ValueString(); got != "2026-01-15T12:00:00Z" {
		t.Errorf("expected UpdateTime = 2026-01-15T12:00:00Z, got %q", got)
	}
}

func TestConsumerEmit_SetsUpdateTimeFromStartTimeWhenEndTimeIsZero(t *testing.T) {
	startTime := time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC)
	fake := &fakeLineageAPI{
		emitResult: &emitResult{ProcessName: "projects/p/processes/abc"},
		runResult:  &runInfo{StartTime: startTime}, // EndTime is zero
	}
	model := &DataplexJobModel{}

	newTestResource(fake).ConsumerEmit(context.Background(), model)

	if got := model.UpdateTime.ValueString(); got != "2026-01-15T11:00:00Z" {
		t.Errorf("expected UpdateTime from StartTime = 2026-01-15T11:00:00Z, got %q", got)
	}
}

func TestConsumerEmit_ReturnsErrorOnAPIFailure(t *testing.T) {
	fake := &fakeLineageAPI{emitErr: fmt.Errorf("gRPC: permission denied")}

	diags := newTestResource(fake).ConsumerEmit(context.Background(), &DataplexJobModel{})

	if !diags.HasError() {
		t.Error("expected error diagnostics when emitAndCapture fails")
	}
}

// ── ConsumerRead ──────────────────────────────────────────────────────────────

func TestConsumerRead_ReturnsTrueWhenProcessExists(t *testing.T) {
	fake := &fakeLineageAPI{
		processResult: &processInfo{ProcessName: "projects/p/processes/abc", OriginVerified: true},
	}

	exists, diags := newTestResource(fake).ConsumerRead(context.Background(), modelWithProcessName("projects/p/processes/abc"))

	if !exists {
		t.Error("expected exists=true when process is found")
	}
	if diags.HasError() {
		t.Errorf("unexpected error: %v", diags)
	}
}

func TestConsumerRead_CallsGetProcessWithStoredName(t *testing.T) {
	fake := &fakeLineageAPI{
		processResult: &processInfo{ProcessName: "projects/p/processes/abc"},
	}

	newTestResource(fake).ConsumerRead(context.Background(), modelWithProcessName("projects/p/processes/abc"))

	if len(fake.processCalls) != 1 || fake.processCalls[0] != "projects/p/processes/abc" {
		t.Errorf("expected getProcess(%q), got calls: %v", "projects/p/processes/abc", fake.processCalls)
	}
}

func TestConsumerRead_ReturnsFalseWhenProcessIsGone(t *testing.T) {
	// nil processResult = 404 = drift — not an error, but resource must be re-created.
	fake := &fakeLineageAPI{processResult: nil}

	exists, diags := newTestResource(fake).ConsumerRead(context.Background(), modelWithProcessName("projects/p/processes/abc"))

	if exists {
		t.Error("expected exists=false when getProcess returns nil (404)")
	}
	if diags.HasError() {
		t.Error("404 (drift) must not produce an error diagnostic")
	}
}

func TestConsumerRead_ReturnsTrueWithoutCallingAPIWhenProcessNameEmpty(t *testing.T) {
	// Empty process_name means the resource has never been applied yet.
	// ConsumerRead must be a no-op and return exists=true.
	fake := &fakeLineageAPI{}

	exists, diags := newTestResource(fake).ConsumerRead(context.Background(), &DataplexJobModel{})

	if !exists {
		t.Error("expected exists=true when ProcessName is empty (not yet applied)")
	}
	if diags.HasError() {
		t.Errorf("unexpected error: %v", diags)
	}
	if len(fake.processCalls) != 0 {
		t.Errorf("expected getProcess NOT called, got %d calls", len(fake.processCalls))
	}
}

func TestConsumerRead_ReturnsErrorOnAPIFailure(t *testing.T) {
	fake := &fakeLineageAPI{processErr: fmt.Errorf("gRPC: unavailable")}

	exists, diags := newTestResource(fake).ConsumerRead(context.Background(), modelWithProcessName("projects/p/processes/abc"))

	if exists {
		t.Error("expected exists=false on API error")
	}
	if !diags.HasError() {
		t.Error("expected error diagnostics on API failure")
	}
}

func TestConsumerRead_CallsRefreshRunStateAfterSuccessfulRead(t *testing.T) {
	fake := &fakeLineageAPI{
		processResult: &processInfo{ProcessName: "projects/p/processes/abc", OriginVerified: true},
	}

	newTestResource(fake).ConsumerRead(context.Background(), modelWithProcessName("projects/p/processes/abc"))

	if len(fake.runCalls) == 0 {
		t.Error("expected getLatestRun to be called after successful read (refreshRunState)")
	}
}

// ── ConsumerDelete ────────────────────────────────────────────────────────────

func TestConsumerDelete_CallsDeleteWithCorrectName(t *testing.T) {
	fake := &fakeLineageAPI{}

	diags := newTestResource(fake).ConsumerDelete(context.Background(), modelWithProcessName("projects/p/processes/abc"))

	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if len(fake.deleteCalls) != 1 || fake.deleteCalls[0] != "projects/p/processes/abc" {
		t.Errorf("expected deleteProcess(%q), got calls: %v", "projects/p/processes/abc", fake.deleteCalls)
	}
}

func TestConsumerDelete_IsNoopWhenProcessNameEmpty(t *testing.T) {
	fake := &fakeLineageAPI{}

	diags := newTestResource(fake).ConsumerDelete(context.Background(), &DataplexJobModel{})

	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if len(fake.deleteCalls) != 0 {
		t.Errorf("expected deleteProcess NOT called, got %d calls", len(fake.deleteCalls))
	}
}

func TestConsumerDelete_IsNoopWhenClientIsNil(t *testing.T) {
	// nil client with a non-empty ProcessName must not panic — guard in ConsumerDelete.
	r := newTestResource(nil)

	diags := r.ConsumerDelete(context.Background(), modelWithProcessName("projects/p/processes/abc"))

	if diags.HasError() {
		t.Fatalf("unexpected error with nil client: %v", diags)
	}
}

func TestConsumerDelete_ReturnsErrorOnAPIFailure(t *testing.T) {
	fake := &fakeLineageAPI{deleteErr: fmt.Errorf("permission denied")}

	diags := newTestResource(fake).ConsumerDelete(context.Background(), modelWithProcessName("projects/p/processes/abc"))

	if !diags.HasError() {
		t.Error("expected error diagnostics when deleteProcess fails")
	}
}

