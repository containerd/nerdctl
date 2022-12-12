/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package utils

import (
	"fmt"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// AddStringFlag is similar to cmd.Flags().String but supports aliases and env var
func AddStringFlag(cmd *cobra.Command, name string, aliases []string, value string, env, usage string) {
	if env != "" {
		usage = fmt.Sprintf("%s [$%s]", usage, env)
	}
	if envV, ok := os.LookupEnv(env); ok {
		value = envV
	}
	aliasesUsage := fmt.Sprintf("Alias of --%s", name)
	p := new(string)
	flags := cmd.Flags()
	flags.StringVar(p, name, value, usage)
	for _, a := range aliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			flags.StringVarP(p, a, a, value, aliasesUsage)
		} else {
			flags.StringVar(p, a, value, aliasesUsage)
		}
	}
}

// AddPersistentStringFlag is similar to AddStringFlag but persistent.
// See https://github.com/spf13/cobra/blob/main/user_guide.md#persistent-flags to learn what is "persistent".
func AddPersistentStringFlag(cmd *cobra.Command, name string, aliases, localAliases, persistentAliases []string, aliasToBeInherited *pflag.FlagSet, value string, env, usage string) {
	if env != "" {
		usage = fmt.Sprintf("%s [$%s]", usage, env)
	}
	if envV, ok := os.LookupEnv(env); ok {
		value = envV
	}
	aliasesUsage := fmt.Sprintf("Alias of --%s", name)
	p := new(string)

	// flags is full set of flag(s)
	// flags can redefine alias already used in subcommands
	flags := cmd.Flags()
	for _, a := range aliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			flags.StringVarP(p, a, a, value, aliasesUsage)
		} else {
			flags.StringVar(p, a, value, aliasesUsage)
		}
		// non-persistent flags are not added to the InheritedFlags, so we should add them manually
		f := flags.Lookup(a)
		aliasToBeInherited.AddFlag(f)
	}

	// localFlags are local to the rootCmd
	localFlags := cmd.LocalFlags()
	for _, a := range localAliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			localFlags.StringVarP(p, a, a, value, aliasesUsage)
		} else {
			localFlags.StringVar(p, a, value, aliasesUsage)
		}
	}

	// persistentFlags cannot redefine alias already used in subcommands
	persistentFlags := cmd.PersistentFlags()
	persistentFlags.StringVar(p, name, value, usage)
	for _, a := range persistentAliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			persistentFlags.StringVarP(p, a, a, value, aliasesUsage)
		} else {
			persistentFlags.StringVar(p, a, value, aliasesUsage)
		}
	}
}

// AddPersistentBoolFlag is similar to AddBoolFlag but persistent.
// See https://github.com/spf13/cobra/blob/main/user_guide.md#persistent-flags to learn what is "persistent".
func AddPersistentBoolFlag(cmd *cobra.Command, name string, aliases, nonPersistentAliases []string, value bool, env, usage string) {
	if env != "" {
		usage = fmt.Sprintf("%s [$%s]", usage, env)
	}
	if envV, ok := os.LookupEnv(env); ok {
		var err error
		value, err = strconv.ParseBool(envV)
		if err != nil {
			logrus.WithError(err).Warnf("Invalid boolean value for `%s`", env)
		}
	}
	aliasesUsage := fmt.Sprintf("Alias of --%s", name)
	p := new(bool)
	flags := cmd.Flags()
	for _, a := range nonPersistentAliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			flags.BoolVarP(p, a, a, value, aliasesUsage)
		} else {
			flags.BoolVar(p, a, value, aliasesUsage)
		}
	}

	persistentFlags := cmd.PersistentFlags()
	persistentFlags.BoolVar(p, name, value, usage)
	for _, a := range aliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			persistentFlags.BoolVarP(p, a, a, value, aliasesUsage)
		} else {
			persistentFlags.BoolVar(p, a, value, aliasesUsage)
		}
	}
}

// AddPersistentStringArrayFlag is similar to cmd.Flags().StringArray but supports aliases and env var and persistent.
// See https://github.com/spf13/cobra/blob/main/user_guide.md#persistent-flags to learn what is "persistent".
func AddPersistentStringArrayFlag(cmd *cobra.Command, name string, aliases, nonPersistentAliases []string, value []string, env string, usage string) {
	if env != "" {
		usage = fmt.Sprintf("%s [$%s]", usage, env)
	}
	if envV, ok := os.LookupEnv(env); ok {
		value = []string{envV}
	}
	aliasesUsage := fmt.Sprintf("Alias of --%s", name)
	p := new([]string)
	flags := cmd.Flags()
	for _, a := range nonPersistentAliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			flags.StringArrayVarP(p, a, a, value, aliasesUsage)
		} else {
			flags.StringArrayVar(p, a, value, aliasesUsage)
		}
	}

	persistentFlags := cmd.PersistentFlags()
	persistentFlags.StringArrayVar(p, name, value, usage)
	for _, a := range aliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			persistentFlags.StringArrayVarP(p, a, a, value, aliasesUsage)
		} else {
			persistentFlags.StringArrayVar(p, a, value, aliasesUsage)
		}
	}
}

func CheckExperimental(feature string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		experimental, err := cmd.Flags().GetBool("experimental")
		if err != nil {
			return err
		}
		if !experimental {
			return fmt.Errorf("%s is experimental feature, you should enable experimental config", feature)
		}
		return nil
	}
}

// IsExactArgs returns an error if there is not the exact number of args
func IsExactArgs(number int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == number {
			return nil
		}
		return fmt.Errorf(
			"%q requires exactly %d %s.\nSee '%s --help'.\n\nUsage:  %s\n\n%s",
			cmd.CommandPath(),
			number,
			"argument(s)",
			cmd.CommandPath(),
			cmd.UseLine(),
			cmd.Short,
		)
	}
}

func GlobalFlags(cmd *cobra.Command) (string, []string) {
	args0, err := os.Executable()
	if err != nil {
		logrus.WithError(err).Warnf("cannot call os.Executable(), assuming the executable to be %q", os.Args[0])
		args0 = os.Args[0]
	}
	if len(os.Args) < 2 {
		return args0, nil
	}

	rootCmd := cmd.Root()
	flagSet := rootCmd.Flags()
	args := []string{}
	flagSet.VisitAll(func(f *pflag.Flag) {
		key := f.Name
		val := f.Value.String()
		if f.Changed {
			args = append(args, "--"+key+"="+val)
		}
	})
	return args0, args
}
