package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mamorett/PhotoLib/internal/exitcode"
	"github.com/mamorett/PhotoLib/internal/output"
	"github.com/mamorett/PhotoLib/internal/planner"
)

// envelope is the standard JSON document for every command.
type envelope struct {
	Photolib int         `json:"qsync"`
	Command  string      `json:"command"`
	OK       bool        `json:"ok"`
	ExitCode int         `json:"exit_code"`
	Warnings []string    `json:"warnings"`
	Data     interface{} `json:"data,omitempty"`
	Error    *jsonError  `json:"error,omitempty"`
}

type jsonError struct {
	Message   string             `json:"message"`
	Conflicts []planner.Conflict `json:"conflicts,omitempty"`
}

// emitJSON writes a JSON envelope to stdout with a trailing newline.
func emitJSON(w io.Writer, command string, code exitcode.ExitCode, warnings []string, data interface{}, errMsg string, conflicts []planner.Conflict) error {
	env := envelope{
		Photolib: 1,
		Command:  command,
		OK:       code == exitcode.Success,
		ExitCode: int(code),
		Warnings: warnings,
		Data:     data,
	}
	if env.Warnings == nil {
		env.Warnings = []string{}
	}
	if errMsg != "" {
		env.Error = &jsonError{Message: errMsg, Conflicts: conflicts}
		env.OK = false
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}

// renderPlanHuman prints a plan in human form. limit caps listed entries unless
// showAll is set.
func renderPlanHuman(w io.Writer, p *planner.Plan, showAll bool) {
	fmt.Fprintf(w, "Plan: %s from %s %s %s\n", p.Direction, p.Source, output.SymbolDirection, p.Dest)

	limit := 50
	shown := 0
	for _, c := range p.Changes {
		if !showAll && shown >= limit {
			fmt.Fprintf(w, "  ... (%d more; use --all)\n", len(p.Changes)-shown)
			break
		}
		var sym output.Symbol
		var note string
		switch c.Kind {
		case planner.ChangeAdd:
			sym = output.SymbolAdd
			note = fmt.Sprintf("(%s, %s)", c.Reason, output.HumanizeIEC(c.Size))
		case planner.ChangeUpdate:
			sym = output.SymbolUpdate
			note = fmt.Sprintf("(%s, %s)", c.Reason, output.HumanizeIEC(c.Size))
		case planner.ChangeDelete:
			sym = output.SymbolDelete
			note = "(staged for purge)"
		}
		fmt.Fprintf(w, "  %s %-40s %s\n", sym, c.Path, note)
		shown++
	}
	for i, cf := range p.Conflicts {
		if !showAll && i >= limit {
			break
		}
		fmt.Fprintf(w, "  %s %-40s (CONFLICT: %s)\n", output.SymbolConflict, cf.Path, cf.Detail)
	}
	fmt.Fprintf(w, "Summary: %d add, %d update, %d deletion staged, %d conflict, %s to transfer\n",
		p.Stats.Additions, p.Stats.Updates, p.Stats.Deletions, p.Stats.Conflicts, output.HumanizeIEC(p.Stats.BytesTotal))
}

// planExit returns the exit code + sentinel for a computed plan.
func planExit(p *planner.Plan) (exitcode.ExitCode, error) {
	if p.Stats.Conflicts > 0 {
		return exitcode.Conflicts, ErrConflicts
	}
	if len(p.Changes) > 0 {
		return exitcode.PendingChanges, ErrPending
	}
	return exitcode.Success, nil
}
