package core

import "github.com/ysqss/shortlink/internal/model"

type Service interface {
	Create(opts model.CreateOptions, longURL string) (*model.ShortLink, error)
	Lookup(slug string) (*model.ShortLink, error)
	Delete(slug string) error
	List(page, pageSize int) ([]*model.ShortLink, int64, error)
	Search(query string) ([]*model.ShortLink, error)
	Update(slug string, opts model.CreateOptions) (*model.ShortLink, error)
	VerifyPassword(slug string, password string) (bool, error)
}
