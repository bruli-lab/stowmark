//nolint:dupl // Both key rotation commands are declared with the same structure on purpose.
package cli

import (
	"fmt"

	"github.com/bruli-lab/go-core/cqs"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/spf13/cobra"
)

type keyFlag struct {
	name, usage string
}

type keyRotation struct {
	use, short, failure string
	first, second       keyFlag
	handler             func(repo *bootstrap.Repository) cqs.CommandHandler
	command             func(repositoryPath, first, second string) cqs.Command
}

func newKeyReKeyCommand() *cobra.Command {
	return newKeyRotationCommand(&keyRotation{
		use:     "rekey",
		short:   "Rotate symmetric key and rewrap all objects",
		failure: "failed to rekey",
		first:   keyFlag{name: "private-key", usage: "private key path"},
		second:  keyFlag{name: "public-key", usage: "public key path"},
		handler: (*bootstrap.Repository).RekeyKey,
		command: func(repositoryPath, privateKey, publicKey string) cqs.Command {
			return app.RekeyKeyCommand{RepositoryPath: repositoryPath, PrivateKeyPath: privateKey, PublicKeyPath: publicKey}
		},
	})
}

func newKeyRewrapCommand() *cobra.Command {
	return newKeyRotationCommand(&keyRotation{
		use:     "rewrap",
		short:   "Rotate encryption key pair",
		failure: "failed to rewrap",
		first:   keyFlag{name: "old-private-key", usage: "old private key path"},
		second:  keyFlag{name: "new-public-key", usage: "new public key path"},
		handler: (*bootstrap.Repository).RewrapKey,
		command: func(repositoryPath, oldPrivateKey, newPublicKey string) cqs.Command {
			return app.RewrapKeyCommand{RepositoryPath: repositoryPath, OldPrivateKey: oldPrivateKey, NewPublicKey: newPublicKey}
		},
	})
}

func newKeyRotationCommand(rotation *keyRotation) *cobra.Command {
	var repositoryPath, first, second string
	cmd := &cobra.Command{
		Use:   rotation.use,
		Short: rotation.short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				if _, err := rotation.handler(repo).Handle(cmd.Context(), rotation.command(repo.Path(), first, second)); err != nil {
					return fmt.Errorf("%s: %w", rotation.failure, err)
				}
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "rotate encryption key pair successful")
				return err
			})
		},
	}

	cmd.Flags().StringVar(&repositoryPath, "repo", "", "Repository path")
	cmd.Flags().StringVar(&first, rotation.first.name, "", rotation.first.usage)
	cmd.Flags().StringVar(&second, rotation.second.name, "", rotation.second.usage)
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired(rotation.first.name)
	_ = cmd.MarkFlagRequired(rotation.second.name)
	return cmd
}
