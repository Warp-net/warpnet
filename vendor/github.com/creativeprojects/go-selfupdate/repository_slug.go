package selfupdate

import (
	"strings"
)

type RepositorySlug struct {
	owner string
	repo  string
}

// Repository interface
var _ Repository = RepositorySlug{}

// ParseSlug is used to take a string "owner/repo" to make a RepositorySlug
func ParseSlug(slug string) RepositorySlug {
	var owner, repo string
	couple := strings.Split(slug, "/")
	if len(couple) != 2 {
		// give it another try
		couple = strings.Split(slug, "%2F")
	}
	if len(couple) == 2 {
		owner = couple[0]
		repo = couple[1]
	}
	return RepositorySlug{
		owner: owner,
		repo:  repo,
	}
}

// NewRepositorySlug creates a RepositorySlug from owner and repo parameters
func NewRepositorySlug(owner, repo string) RepositorySlug {
	return RepositorySlug{
		owner: owner,
		repo:  repo,
	}
}

func (r RepositorySlug) GetSlug() (string, string, error) {
	if r.owner == "" && r.repo == "" {
		return "", "", ErrInvalidSlug
	}
	if r.owner == "" {
		return r.owner, r.repo, ErrIncorrectParameterOwner
	}
	if r.repo == "" {
		return r.owner, r.repo, ErrIncorrectParameterRepo
	}
	return r.owner, r.repo, nil
}

func (r RepositorySlug) Get() (interface{}, error) {
	_, _, err := r.GetSlug()
	if err != nil {
		return "", err
	}
	return r.owner + "/" + r.repo, nil
}
