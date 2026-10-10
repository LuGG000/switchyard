package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/state"
)

// decisionSchema is the version of the `decision --json` output.
const decisionSchema = 1

type decisionReport struct {
	Schema  int              `json:"schema"`
	Pending *pendingDecision `json:"pending"`
}

// pendingDecision is a limit waiting for an answer from the mod's buttons.
type pendingDecision struct {
	Profile   string    `json:"profile"`
	Reason    string    `json:"reason"`
	Options   []string  `json:"options"`
	Carry     bool      `json:"carry"`
	ExpiresAt time.Time `json:"expires_at"`
}

func newDecisionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "decision",
		Short: "The question the launcher puts to the mod's buttons at a limit",
		Long: "Show the limit that waits for an answer, if there is one. The mod polls this command\n" +
			"every few seconds; each call also tells the launcher that a mod is present, which makes\n" +
			"it keep claude running at a limit in ask mode and wait for the buttons instead of asking\n" +
			"in the terminal. Without an answer in time the terminal question follows.\n\n" +
			"Answer with: switchyard decision answer switch <profile> [--resume|--fresh]\n" +
			"          or: switchyard decision answer stay",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStateStore()
			if err != nil {
				return err
			}
			now := time.Now()
			_ = store.TouchHeartbeat(now)
			st, err := store.Read()
			if err != nil {
				return err
			}
			report := decisionReport{Schema: decisionSchema}
			if d := st.Decision; d != nil && d.Answer == nil && now.Before(d.ExpiresAt) {
				report.Pending = &pendingDecision{Profile: d.Profile, Reason: d.Reason, Options: d.Options, Carry: d.Carry, ExpiresAt: d.ExpiresAt}
			}
			if asJSON {
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return err
				}
				println(cmd, string(data))
				return nil
			}
			if report.Pending == nil {
				println(cmd, "No limit waits for an answer.")
				return nil
			}
			printf(cmd, "%s reached its limit; options: %v\n", report.Pending.Profile, report.Pending.Options)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON (schema 1)")
	cmd.AddCommand(newDecisionAnswerCmd())
	return cmd
}

func newDecisionAnswerCmd() *cobra.Command {
	var resume, fresh bool
	cmd := &cobra.Command{
		Use:   "answer <switch <profile> | stay>",
		Short: "Answer the limit that waits for a decision",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			answer := state.Answer{Action: args[0]}
			switch args[0] {
			case state.ActionSwitch:
				if len(args) != 2 {
					return errors.New("switch needs a profile name")
				}
				answer.Target = args[1]
				if resume || fresh {
					carry := resume
					answer.Carry = &carry
				}
			case state.ActionStay:
				if len(args) != 1 {
					return errors.New("stay takes no profile")
				}
			default:
				return fmt.Errorf("unknown answer %q (want switch or stay)", args[0])
			}
			store, err := newStateStore()
			if err != nil {
				return err
			}
			err = store.Update(func(st *state.State) error {
				d := st.Decision
				if d == nil || d.Answer != nil || !time.Now().Before(d.ExpiresAt) {
					return errors.New("no limit is waiting for an answer")
				}
				if answer.Action == state.ActionSwitch && !slices.Contains(d.Options, answer.Target) {
					return fmt.Errorf("%s is not an option (want one of %v)", answer.Target, d.Options)
				}
				d.Answer = &answer
				return nil
			})
			if err != nil {
				return err
			}
			printf(cmd, "Answered: %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&resume, "resume", false, "continue the conversation in the new profile")
	cmd.Flags().BoolVar(&fresh, "fresh", false, "start a new conversation")
	cmd.MarkFlagsMutuallyExclusive("resume", "fresh")
	return cmd
}
