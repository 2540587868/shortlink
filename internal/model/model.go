package model

import (
	"errors"
	"time"
)

var (
	ErrSlugExists   = errors.New("slug already exists")
	ErrSlugNotFound = errors.New("slug not found")
	ErrLinkExpired  = errors.New("link expired")
	ErrInvalidSlug  = errors.New("invalid slug format")
)

type ShortLink struct {
	ID        int64
	Slug      string
	LongURL   string
	Password  string
	ExpiresAt *time.Time
	IsDeleted bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateOptions struct {
	CustomSlug string
	Password   string
	TTL        time.Duration
}

type ClickEvent struct {
	Slug      string
	Timestamp time.Time
	Referer   string
	UserAgent string
	IP        string
}
