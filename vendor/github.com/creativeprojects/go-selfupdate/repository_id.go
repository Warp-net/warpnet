package selfupdate

type RepositoryID int

// Repository interface
var _ Repository = RepositoryID(0)

// NewRepositoryID creates a repository ID from an integer
func NewRepositoryID(id int) RepositoryID {
	return RepositoryID(id)
}

func (r RepositoryID) GetSlug() (string, string, error) {
	return "", "", ErrInvalidID
}

func (r RepositoryID) Get() (interface{}, error) {
	return int(r), nil
}
