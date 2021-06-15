package bugutil

import (
	"github.com/zpnk/go-bitly"
)

func ShortenURL(url string, token string) string {
	b := bitly.New(token)
	shortURL, err := b.Links.Shorten(url)
	if err != nil {
		return url
	}
	return shortURL.URL
}
