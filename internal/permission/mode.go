package permission

import "fmt"

// Mode represents a permission trust level.
type Mode string

const (
	ModeDefault    Mode = "default"
	ModeAutoAccept Mode = "auto-accept"
	ModeYolo       Mode = "yolo"
)

// Valid returns true if the mode is one of the three defined constants.
func (m Mode) Valid() bool {
	switch m {
	case ModeDefault, ModeAutoAccept, ModeYolo:
		return true
	default:
		return false
	}
}

// Rule maps a tool name pattern to an action type, with an optional
// destructive filter. Rules are evaluated in order (first match wins).
type Rule struct {
	Tool        string     // tool name to match, e.g. "file_read", "bash", "*"
	Action      ActionType // what to do when matched
	Destructive *bool      // if non-nil, only match when action.Destructive equals this value
}

// ActionType is the action a rule prescribes.
type ActionType string

const (
	ActionAllow ActionType = "allow"
	ActionAsk   ActionType = "ask"
	ActionDeny  ActionType = "deny"
)

// Config holds the permission configuration for the evaluator.
type Config struct {
	Mode  Mode
	Rules []Rule // custom rules (unused in Phase 1, reserved for Story 3.1)
}

// DefaultConfig returns a Config with default mode and no custom rules.
func DefaultConfig() Config {
	return Config{
		Mode:  ModeDefault,
		Rules: nil,
	}
}

// Validate checks that the config has a valid mode.
func (c Config) Validate() error {
	if !c.Mode.Valid() {
		return fmt.Errorf("permission: invalid mode %q", c.Mode)
	}
	return nil
}
