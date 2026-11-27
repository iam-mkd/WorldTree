package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/iam-mkd/WorldTree/internal/envfile"
	"github.com/iam-mkd/WorldTree/internal/world"
	"github.com/spf13/cobra"
)

func newEnvCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "env [dir]",
		Short: "Print the world's environment: ports, DATABASE_URL, Spring datasource and Redis settings",
		Long: `env prints the environment 'worldtree up' generated for the world of dir (default: current
directory), read from <dir>/.worldtree/env:

  WORLDTREE_ID, WORLDTREE_PROJECT
  WORLDTREE_PORT_<SERVICE>_<TARGET>        leased host port of every published port
  DATABASE_URL, SPRING_DATASOURCE_URL/USERNAME/PASSWORD   first Postgres service
  REDIS_URL, SPRING_DATA_REDIS_HOST/PORT                  first Redis/Valkey service

Load it into a shell (Spring Boot reads SPRING_* names only from real environment variables):

  eval "$(worldtree env --format sh)"                              # bash, zsh
  worldtree env --format powershell | Out-String | Invoke-Expression  # PowerShell

The output contains the database password.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(filepath.Join(abs, filepath.FromSlash(world.EnvPath)))
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("no world environment in %s: run `worldtree up` there first", abs)
			}
			if err != nil {
				return err
			}
			vars, err := envfile.Parse(data)
			if err != nil {
				return err
			}
			out, err := envfile.Format(vars, format)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), out)
			return err
		},
	}
	cmd.Flags().StringVar(&format, "format", envfile.Dotenv, "output format: dotenv, sh or powershell")
	return cmd
}
