package config

type Ref struct {
	Kind  string `yaml:"kind" json:"kind"`
	Value string `yaml:"value" json:"value"`
}

type Skill struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Source      string `yaml:"source,omitempty" json:"source,omitempty"`
	SourcePath  string `yaml:"source_path,omitempty" json:"source_path,omitempty"`
	Ref         *Ref   `yaml:"ref,omitempty" json:"ref,omitempty"`
	Resolved    string `yaml:"resolved,omitempty" json:"resolved,omitempty"`
	Hash        string `yaml:"hash,omitempty" json:"hash,omitempty"`
}

type Library struct {
	Skills []Skill `yaml:"skills" json:"skills"`
}

type Preset struct {
	Name   string   `yaml:"name" json:"name"`
	Skills []string `yaml:"skills,omitempty" json:"skills,omitempty"`
}

type Binding struct {
	Presets []string `yaml:"presets,omitempty" json:"presets,omitempty"`
	Skills  []string `yaml:"skills,omitempty" json:"skills,omitempty"`
}

type Project struct {
	Name          string   `yaml:"name" json:"name"`
	Path          string   `yaml:"path" json:"path"`
	Agents        []string `yaml:"agents" json:"agents"`
	Binding       `yaml:",inline" json:",inline"`
	AgentBindings map[string]Binding `yaml:"agent_bindings,omitempty" json:"agent_bindings,omitempty"`
}

type OperationKind string

const (
	OperationCleanup OperationKind = "cleanup"
	OperationAdopt   OperationKind = "adopt"
	// OperationReconcile is an authenticated request to converge one exact
	// overlay path back to the skill recorded in the durable config.
	OperationReconcile OperationKind = "reconcile"
)

type Scope struct {
	Project     string `yaml:"project,omitempty" json:"project,omitempty"`
	ProjectPath string `yaml:"project_path,omitempty" json:"project_path,omitempty"`
	Agent       string `yaml:"agent,omitempty" json:"agent,omitempty"`
}

type Fingerprint struct {
	Kind       string `yaml:"kind" json:"kind"`
	Hash       string `yaml:"hash,omitempty" json:"hash,omitempty"`
	LinkTarget string `yaml:"link_target,omitempty" json:"link_target,omitempty"`
}

type TransactionPhase string

const (
	TransactionForward        TransactionPhase = "forward"
	TransactionRollbackSource TransactionPhase = "rollback-source"
	TransactionRollback       TransactionPhase = "rollback"
)

type PendingOperation struct {
	ID       string        `yaml:"id" json:"id"`
	Kind     OperationKind `yaml:"kind" json:"kind"`
	Scope    Scope         `yaml:"scope" json:"scope"`
	Target   string        `yaml:"target" json:"target"`
	SkillID  string        `yaml:"skill_id" json:"skill_id"`
	Reason   string        `yaml:"reason,omitempty" json:"reason,omitempty"`
	Temp     string        `yaml:"temp,omitempty" json:"temp,omitempty"`
	Backup   string        `yaml:"backup,omitempty" json:"backup,omitempty"`
	Original *Fingerprint  `yaml:"original,omitempty" json:"original,omitempty"`
	// JournalHash binds an adopt delete manifest to the operation authorized
	// before any user content is moved. Recovery must fail closed if it differs.
	JournalHash string `yaml:"journal_hash,omitempty" json:"journal_hash,omitempty"`
	// TransactionID and TransactionPhase group link operations checkpointed
	// before the first filesystem action. Recovery follows this durable
	// direction rather than inferring intent from a partially changed overlay.
	TransactionID    string           `yaml:"transaction_id,omitempty" json:"transaction_id,omitempty"`
	TransactionPhase TransactionPhase `yaml:"transaction_phase,omitempty" json:"transaction_phase,omitempty"`
	// Reconcile is authorized only for the exact object observed when the
	// transaction was checkpointed. ExpectedAbsent may additionally authorize
	// creating a missing target after an interrupted cleanup.
	ExpectedSkillID   string            `yaml:"expected_skill_id,omitempty" json:"expected_skill_id,omitempty"`
	Expected          *Fingerprint      `yaml:"expected,omitempty" json:"expected,omitempty"`
	ExpectedAbsent    bool              `yaml:"expected_absent,omitempty" json:"expected_absent,omitempty"`
	Tombstone         string            `yaml:"tombstone,omitempty" json:"tombstone,omitempty"`
	Rollback          *PendingOperation `yaml:"rollback,omitempty" json:"rollback,omitempty"`
	ParentOperationID string            `yaml:"parent_operation_id,omitempty" json:"parent_operation_id,omitempty"`
}

type Config struct {
	Library           Library            `yaml:"library" json:"library"`
	Presets           []Preset           `yaml:"presets,omitempty" json:"presets,omitempty"`
	Agents            map[string]Binding `yaml:"agents,omitempty" json:"agents,omitempty"`
	Projects          []Project          `yaml:"projects,omitempty" json:"projects,omitempty"`
	PendingOperations []PendingOperation `yaml:"pending_operations,omitempty" json:"pending_operations,omitempty"`
}
