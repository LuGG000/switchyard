package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/doctor"
	"github.com/LuGG000/switchyard/internal/profiles"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check profiles, logins, settings and shared links for problems",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			source, err := cfg.ResolveSourceDir()
			if err != nil {
				return err
			}
			dataDir, err := config.DataDir()
			if err != nil {
				return err
			}
			claude, err := exec.LookPath("claude")
			if err != nil {
				return errors.New("claude executable not found in PATH")
			}
			d := &doctor.Doctor{
				Profiles:  &profiles.Manager{Root: filepath.Join(dataDir, profilesSubdir), Claude: claude, Environ: os.Environ()},
				Config:    cfg,
				SourceDir: source,
				Environ:   os.Environ(),
			}
			findings, err := d.Run(cmd.Context())
			if err != nil {
				return err
			}
			for _, f := range findings {
				printf(cmd, "%-5s %s: %s\n", f.Level, f.Subject, f.Message)
			}
			if doctor.Worst(findings) == doctor.Error {
				return exitError{code: 1}
			}
			return nil
		},
	}
}
