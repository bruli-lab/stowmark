//go:build integration

package stowmark_test

import "github.com/bruli-lab/stowmark/internal/domain/repository"

func Versions() map[string]int {
	return map[string]int{
		"1": repository.FormatVersionOne.Int(),
		"2": repository.FormatVersionTwo.Int(),
	}
}
