package dataplex

import (
	"testing"

	lineagepb "cloud.google.com/go/datacatalog/lineage/apiv1/lineagepb"
	"google.golang.org/protobuf/types/known/structpb"
)


// ── matchesOrigin (pure function) ────────────────────────────────────────────

func TestMatchesOrigin_MatchesByProviderOriginAndDisplayName(t *testing.T) {
	process := &lineagepb.Process{
		DisplayName: "my-namespace:my-job",
		Origin:      &lineagepb.Origin{SourceType: lineagepb.Origin_CUSTOM, Name: providerProducer},
	}

	if !matchesOrigin(process, "my-namespace", "my-job") {
		t.Error("expected match when origin + display_name both match")
	}
	if matchesOrigin(process, "wrong-namespace", "my-job") {
		t.Error("expected no match when namespace differs")
	}
	if matchesOrigin(process, "my-namespace", "wrong-job") {
		t.Error("expected no match when job name differs")
	}
}

func TestMatchesOrigin_MatchesByBareJobName(t *testing.T) {
	process := &lineagepb.Process{
		DisplayName: "my-job",
		Origin:      &lineagepb.Origin{SourceType: lineagepb.Origin_CUSTOM, Name: providerProducer},
	}

	if !matchesOrigin(process, "any-namespace", "my-job") {
		t.Error("expected match when display_name equals bare job name")
	}
}

func TestMatchesOrigin_FallsBackToDisplayNameWhenOriginDiffers(t *testing.T) {
	process := &lineagepb.Process{
		DisplayName: "ns:job",
		Origin:      &lineagepb.Origin{SourceType: lineagepb.Origin_CUSTOM, Name: "other-tool"},
	}

	if !matchesOrigin(process, "ns", "job") {
		t.Error("expected display_name fallback when origin doesn't match provider")
	}
}

func TestMatchesOrigin_MatchesByOLAttributes(t *testing.T) {
	process := &lineagepb.Process{
		Attributes: map[string]*structpb.Value{
			"openlineage_namespace": structpb.NewStringValue("attr-ns"),
			"openlineage_job":       structpb.NewStringValue("attr-job"),
		},
	}

	if !matchesOrigin(process, "attr-ns", "attr-job") {
		t.Error("expected match via OL attributes")
	}
	if matchesOrigin(process, "attr-ns", "wrong-job") {
		t.Error("expected no match when attribute job name differs")
	}
}

func TestMatchesOrigin_NoOriginNoAttributes_FallsBackToDisplayName(t *testing.T) {
	process := &lineagepb.Process{DisplayName: "ns:job"}

	if !matchesOrigin(process, "ns", "job") {
		t.Error("expected display_name fallback with no origin and no attributes")
	}
	if matchesOrigin(process, "ns", "other") {
		t.Error("expected no match when display_name differs")
	}
}

// ── getStringAttribute (pure function) ───────────────────────────────────────

func TestGetStringAttribute_ReturnsStringValue(t *testing.T) {
	attrs := map[string]*structpb.Value{"key": structpb.NewStringValue("value")}

	if got := getStringAttribute(attrs, "key"); got != "value" {
		t.Errorf("expected %q, got %q", "value", got)
	}
}

func TestGetStringAttribute_ReturnsEmptyForMissingKey(t *testing.T) {
	if got := getStringAttribute(map[string]*structpb.Value{}, "missing"); got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}
}

func TestGetStringAttribute_ReturnsEmptyForNumberValue(t *testing.T) {
	attrs := map[string]*structpb.Value{"n": structpb.NewNumberValue(42)}

	if got := getStringAttribute(attrs, "n"); got != "" {
		t.Errorf("expected empty string for number value, got %q", got)
	}
}

func TestGetStringAttribute_ReturnsEmptyForBoolValue(t *testing.T) {
	attrs := map[string]*structpb.Value{"b": structpb.NewBoolValue(true)}

	if got := getStringAttribute(attrs, "b"); got != "" {
		t.Errorf("expected empty string for bool value, got %q", got)
	}
}

func TestGetStringAttribute_ReturnsEmptyForNilEntry(t *testing.T) {
	attrs := map[string]*structpb.Value{"nil-key": nil}

	if got := getStringAttribute(attrs, "nil-key"); got != "" {
		t.Errorf("expected empty string for nil value, got %q", got)
	}
}


