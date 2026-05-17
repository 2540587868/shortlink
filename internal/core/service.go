package core

import (
	"fmt"
	"time"

	"github.com/ysqss/shortlink/internal/bloom"
	"github.com/ysqss/shortlink/internal/cache"
	"github.com/ysqss/shortlink/internal/model"
	"github.com/ysqss/shortlink/internal/slug"
	"github.com/ysqss/shortlink/internal/store"
)

type service struct {
	store *store.Store
	cache *cache.ShardedLRU[string, *model.ShortLink]
	gen   *slug.Generator
	bf    *bloom.Filter
}

func NewService(st *store.Store, c *cache.ShardedLRU[string, *model.ShortLink]) Service {
	return &service{
		store: st,
		cache: c,
		gen:   slug.NewGenerator(),
	}
}

func NewServiceWithBloom(st *store.Store, c *cache.ShardedLRU[string, *model.ShortLink], bf *bloom.Filter) Service {
	return &service{
		store: st,
		cache: c,
		gen:   slug.NewGenerator(),
		bf:    bf,
	}
}

func (s *service) Create(opts model.CreateOptions, longURL string) (*model.ShortLink, error) {
	if longURL == "" {
		return nil, fmt.Errorf("long_url is required")
	}

	var slugStr string
	if opts.CustomSlug != "" {
		if len(opts.CustomSlug) < 1 || len(opts.CustomSlug) > 12 {
			return nil, fmt.Errorf("custom slug must be 1-12 characters")
		}
		if s.bf != nil && !s.bf.Contains(opts.CustomSlug) {
			slugStr = opts.CustomSlug
		} else {
			exists, err := s.store.SlugExists(opts.CustomSlug)
			if err != nil {
				return nil, err
			}
			if exists {
				return nil, model.ErrSlugExists
			}
			slugStr = opts.CustomSlug
		}
	} else {
		for {
			slugStr = s.gen.Generate()
			exists, err := s.store.SlugExists(slugStr)
			if err != nil {
				return nil, err
			}
			if !exists {
				break
			}
		}
	}

	now := time.Now()
	link := &model.ShortLink{
		ID:        s.gen.LastID(),
		Slug:      slugStr,
		LongURL:   longURL,
		Password:  opts.Password,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if opts.TTL > 0 {
		expires := now.Add(opts.TTL)
		link.ExpiresAt = &expires
	}

	if err := s.store.InsertLink(link); err != nil {
		return nil, err
	}

	s.cache.Set(slugStr, link)
	if s.bf != nil {
		s.bf.Add(slugStr)
	}
	return link, nil
}

func (s *service) Lookup(slugStr string) (*model.ShortLink, error) {
	if slugStr == "" {
		return nil, model.ErrInvalidSlug
	}

	if link, ok := s.cache.Get(slugStr); ok {
		if link.IsDeleted {
			return nil, model.ErrSlugNotFound
		}
		if link.ExpiresAt != nil && time.Now().After(*link.ExpiresAt) {
			return nil, model.ErrLinkExpired
		}
		return link, nil
	}

	link, err := s.store.GetLinkBySlug(slugStr)
	if err != nil {
		return nil, model.ErrSlugNotFound
	}

	if link.ExpiresAt != nil && time.Now().After(*link.ExpiresAt) {
		return nil, model.ErrLinkExpired
	}

	s.cache.Set(slugStr, link)
	return link, nil
}

func (s *service) Delete(slugStr string) error {
	link, err := s.Lookup(slugStr)
	if err != nil {
		return err
	}

	if err := s.store.DeleteLink(slugStr); err != nil {
		return err
	}

	link.IsDeleted = true
	link.UpdatedAt = time.Now()
	s.cache.Set(slugStr, link)
	return nil
}

func (s *service) List(page, pageSize int) ([]*model.ShortLink, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.ListLinks(page, pageSize)
}

func (s *service) Search(query string) ([]*model.ShortLink, error) {
	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}
	return s.store.SearchLinks(query)
}

func (s *service) Update(slugStr string, opts model.CreateOptions) (*model.ShortLink, error) {
	link, err := s.Lookup(slugStr)
	if err != nil {
		return nil, err
	}

	if opts.CustomSlug != "" && opts.CustomSlug != slugStr {
		return nil, fmt.Errorf("slug cannot be changed")
	}
	if opts.Password != "" {
		link.Password = opts.Password
	}
	if opts.TTL > 0 {
		expires := time.Now().Add(opts.TTL)
		link.ExpiresAt = &expires
	}

	if err := s.store.UpdateLink(slugStr, link); err != nil {
		return nil, err
	}

	s.cache.Set(slugStr, link)
	return link, nil
}

func (s *service) VerifyPassword(slugStr string, password string) (bool, error) {
	link, err := s.Lookup(slugStr)
	if err != nil {
		return false, err
	}
	if link.Password == "" {
		return true, nil
	}
	return link.Password == password, nil
}
