package mediawiki

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

const MainPagePath = "/wiki/Main_Page"

// PageProblem describes why a page response is not a working wiki page, or returns "" when it is. Retrying makes
// sense for an error status, which a database that is still starting can cause, but not for an incomplete page: PHP
// answers fatal errors with HTTP 200 while it displays errors, and only the missing closing </html> shows them.
func PageProblem(status int, body string) (problem string, retry bool) {
	if status != http.StatusOK {
		return fmt.Sprintf("HTTP %d: %s", status, pageText(body)), true
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "</html>") {
		return "the page is incomplete, most likely because of a PHP fatal error: " + pageText(body), false
	}
	return "", false
}

var (
	htmlTags   = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]+>`)
	whitespace = regexp.MustCompile(`\s+`)
)

func pageText(html string) string {
	text := strings.TrimSpace(whitespace.ReplaceAllString(htmlTags.ReplaceAllString(html, " "), " "))
	if len(text) > 2000 {
		return text[:2000] + "…"
	}
	return text
}
