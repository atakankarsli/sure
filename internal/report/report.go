// Package report defines Sure's versioned, agent-facing output documents and
// the exit codes derived from them.
package report

import (
	"github.com/atakankarsli/sure/internal/contracts"
	"github.com/atakankarsli/sure/internal/evidence"
)

const (
	RunSchema        = "sure.run/v1"
	PlanSchema       = "sure.plan/v1"
	ValidationSchema = "sure.validation/v1"
	DoctorSchema     = "sure.doctor/v1"
	InitSchema       = "sure.init/v1"
)

const (
	ExitClear      = 0
	ExitViolation  = 1
	ExitUsage      = 2
	ExitIncomplete = 3
	ExitNetwork    = 10
	ExitRateLimit  = 11
	ExitTimeout    = 12
	ExitMalformed  = 13
	ExitCanceled   = 130
)

// OperationalPriority orders exit codes when several operational failures
// happen in one run: the first listed wins.
var OperationalPriority = []int{ExitCanceled, ExitTimeout, ExitMalformed, ExitRateLimit, ExitNetwork}

const (
	StatusCompleted       = "completed"
	StatusIncomplete      = "incomplete"
	StatusFailed          = "failed"
	StatusNoChanges       = "no_changes"
	StatusNoApplicable    = "no_applicable_checks"
	GateClear             = "clear"
	GateViolated          = "violated"
	GateIncomplete        = "incomplete"
	GateError             = "error"
	GateNotApplicable     = "not_applicable"
	VerdictSupported      = "supported"
	VerdictViolated       = "violated"
	VerdictUnknown        = "unknown"
	VerdictError          = "error"
	VerdictNotChecked     = "not_checked"
	RoleInspected         = "inspected"
	RoleModelSelected     = "model_selected"
	RegressionIntroduced  = "introduced"
	RegressionPreexisting = "preexisting"
	RegressionUnknown     = "unknown"
	RegressionNotAssessed = "not_assessed"
)

type Run struct {
	SchemaVersion   string             `json:"schema_version"`
	RunID           string             `json:"run_id"`
	Status          string             `json:"status"`
	Gate            string             `json:"gate"`
	ExitCode        int                `json:"exit_code"`
	Confidence      *Confidence        `json:"confidence"`
	Snapshot        *Snapshot          `json:"snapshot"`
	Trust           *Trust             `json:"trust"`
	Provider        *Provider          `json:"provider"`
	Summary         Summary            `json:"summary"`
	Contracts       []ContractEvidence `json:"contracts"`
	Checks          []Check            `json:"checks"`
	Errors          []Error            `json:"errors"`
	UnmappedChanges []string           `json:"unmapped_changes"`
	ContractChanges []contracts.Change `json:"contract_changes"`
	NextActions     []Action           `json:"next_actions"`
	Timing          Timing             `json:"timing"`
}

// Confidence is how sure the decision model is that every selected check
// holds: the lowest P(supported) among them. A selected check the model never
// decided counts as 0.
type Confidence struct {
	Percent     int     `json:"percent"`
	Probability float64 `json:"probability"`
	Basis       string  `json:"basis"`
	Undecided   int     `json:"undecided"`
}

const ConfidenceBasis = "lowest P(supported) across selected checks; undecided checks count as 0"

type Snapshot struct {
	Kind                string   `json:"kind"`
	Scope               string   `json:"scope"`
	Selection           string   `json:"selection"`
	BaseRef             *string  `json:"base_ref"`
	BaseCommit          *string  `json:"base_commit"`
	HeadCommit          *string  `json:"head_commit"`
	SourceDigest        string   `json:"source_digest"`
	ChangedPaths        []string `json:"changed_paths"`
	ApplicationExecuted bool     `json:"application_executed"`
}

type Trust struct {
	Mode            string  `json:"mode"`
	ContractsRef    *string `json:"contracts_ref"`
	ContractsCommit *string `json:"contracts_commit"`
	ContractDigest  string  `json:"contract_digest"`
	PolicyVersion   string  `json:"policy_version"`
	MinProbability  float64 `json:"min_probability"`
}

type Provider struct {
	Name              string   `json:"name"`
	Transport         string   `json:"transport"`
	Endpoint          string   `json:"endpoint"`
	RequestedModel    string   `json:"requested_model"`
	ReturnedModels    []string `json:"returned_models"`
	Mode              string   `json:"mode"`
	PromptVersion     string   `json:"prompt_version"`
	ProbabilitySource string   `json:"probability_source"`
	LongContext       bool     `json:"long_context,omitempty"`
	Usage             Usage    `json:"usage"`
}

type Usage struct {
	Requests             int `json:"requests"`
	LiveRequests         int `json:"live_requests"`
	CacheHits            int `json:"cache_hits"`
	InputTokens          int `json:"input_tokens"`
	OutputTokens         int `json:"output_tokens"`
	EstimatedInputTokens int `json:"estimated_input_tokens"`
}

type Summary struct {
	Total      int `json:"total"`
	Selected   int `json:"selected"`
	Evaluated  int `json:"evaluated"`
	Supported  int `json:"supported"`
	Violated   int `json:"violated"`
	Unknown    int `json:"unknown"`
	Error      int `json:"error"`
	NotChecked int `json:"not_checked"`
}

type ContractEvidence struct {
	ID              string             `json:"id"`
	Adhoc           bool               `json:"adhoc"`
	Path            string             `json:"path"`
	Evidence        string             `json:"evidence"`
	Selected        bool               `json:"selected"`
	SelectedBy      string             `json:"selected_by,omitempty"`
	Files           []evidence.File    `json:"files"`
	Missing         []evidence.Missing `json:"missing"`
	Skipped         []evidence.Missing `json:"skipped"`
	StateBytes      int                `json:"state_bytes"`
	RequestBytes    int                `json:"request_bytes"`
	EstimatedTokens int                `json:"estimated_tokens"`
	Decision        string             `json:"decision"`
	Confidence      *float64           `json:"confidence"`
}

type Probabilities struct {
	Supported float64 `json:"supported"`
	Violated  float64 `json:"violated"`
	Unknown   float64 `json:"unknown"`
}

type Check struct {
	ID                string         `json:"id"`
	Contract          string         `json:"contract"`
	Title             string         `json:"title"`
	Verdict           string         `json:"verdict"`
	Severity          string         `json:"severity"`
	Requirement       string         `json:"requirement"`
	ReasonCode        string         `json:"reason_code"`
	Message           string         `json:"message"`
	MessageOrigin     string         `json:"message_origin"`
	AnalysisLevel     string         `json:"analysis_level"`
	Confidence        *float64       `json:"confidence"`
	Probabilities     *Probabilities `json:"probabilities"`
	ProbabilitySource *string        `json:"probability_source"`
	ModelConfidence   *float64       `json:"model_confidence"`
	Regression        string         `json:"regression"`
	Locations         []Location     `json:"locations"`
	NextActions       []Action       `json:"next_actions"`
}

type Location struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Role      string `json:"role"`
	Excerpt   string `json:"excerpt,omitempty"`
}

const (
	ActionInspectSource  = "inspect_source"
	ActionSupplyContext  = "supply_context"
	ActionNarrowScope    = "narrow_scope"
	ActionProvideVisual  = "provide_visual_evidence"
	ActionReviewContract = "review_contract_change"
	ActionMapChange      = "map_changed_source"
	ActionRecheck        = "recheck"
)

type Action struct {
	Kind        string   `json:"kind"`
	Path        string   `json:"path,omitempty"`
	StartLine   int      `json:"start_line,omitempty"`
	EndLine     int      `json:"end_line,omitempty"`
	Argv        []string `json:"argv,omitempty"`
	Description string   `json:"description,omitempty"`
}

type Error struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Contract string `json:"contract,omitempty"`
	CheckID  string `json:"check_id,omitempty"`
}

type Timing struct {
	CollectionMS int64 `json:"collection_ms"`
	ModelMS      int64 `json:"model_ms"`
	TotalMS      int64 `json:"total_ms"`
}

// Plan is the offline preview of a check run: what would be selected and sent.
type Plan struct {
	SchemaVersion   string             `json:"schema_version"`
	Status          string             `json:"status"`
	ExitCode        int                `json:"exit_code"`
	Snapshot        *Snapshot          `json:"snapshot"`
	Trust           *Trust             `json:"trust"`
	Provider        *Provider          `json:"provider"`
	Summary         PlanSummary        `json:"summary"`
	Contracts       []ContractEvidence `json:"contracts"`
	Outcomes        []PlannedCheck     `json:"checks"`
	Errors          []Error            `json:"errors"`
	UnmappedChanges []string           `json:"unmapped_changes"`
	ContractChanges []contracts.Change `json:"contract_changes"`
	NextActions     []Action           `json:"next_actions"`
}

type PlanSummary struct {
	Total           int `json:"total"`
	Selected        int `json:"selected"`
	WouldEvaluate   int `json:"would_evaluate"`
	KnownUnknown    int `json:"known_unknown"`
	Requests        int `json:"requests"`
	EstimatedTokens int `json:"estimated_tokens"`
}

type PlannedCheck struct {
	ID         string `json:"id"`
	Selected   bool   `json:"selected"`
	Planned    string `json:"planned"`
	ReasonCode string `json:"reason_code,omitempty"`
}

type Validation struct {
	SchemaVersion string              `json:"schema_version"`
	Valid         bool                `json:"valid"`
	ExitCode      int                 `json:"exit_code"`
	Source        string              `json:"source"`
	Contracts     []ValidatedContract `json:"contracts"`
	Issues        []contracts.Issue   `json:"issues"`
	Errors        []Error             `json:"errors"`
}

type ValidatedContract struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Severity string   `json:"severity"`
	Evidence string   `json:"evidence"`
	Checks   []string `json:"checks"`
}

type Doctor struct {
	SchemaVersion string        `json:"schema_version"`
	OK            bool          `json:"ok"`
	ExitCode      int           `json:"exit_code"`
	Checks        []DoctorCheck `json:"checks"`
	Errors        []Error       `json:"errors"`
}

type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type Init struct {
	SchemaVersion string   `json:"schema_version"`
	ExitCode      int      `json:"exit_code"`
	Created       []string `json:"created"`
	Existing      []string `json:"existing"`
	Errors        []Error  `json:"errors"`
}

func Ptr[T any](v T) *T { return &v }

// Percent floors so a value just under a threshold never displays as meeting
// it.
func Percent(p float64) int { return int(p*100 + 1e-9) }
