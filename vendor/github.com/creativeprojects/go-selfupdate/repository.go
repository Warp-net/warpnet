package selfupdate

type Repository interface {
	GetSlug() (string, string, error)
	Get() (interface{}, error)
}
