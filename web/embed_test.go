package web

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedBrowserAssetGraph(t *testing.T) {
	t.Parallel()

	files := Files()
	for _, name := range []string{
		"index.html",
		"app.js",
		"styles.css",
		"assets/ooshdaq-mark.svg",
		"assets/collectible-placeholder.svg",
		"assets/bean-buddies.svg",
	} {
		if _, err := fs.Stat(files, name); err != nil {
			t.Errorf("required browser asset %q is not embedded: %v", name, err)
		}
	}

	moduleImport := regexp.MustCompile(`from\s+"(\.[^"]+)"`)
	styleImport := regexp.MustCompile(`@import url\("(\.[^"]+)"\)`)
	err := fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		extension := path.Ext(name)
		if entry.IsDir() || (extension != ".js" && extension != ".css") {
			return nil
		}
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		matcher := moduleImport
		if extension == ".css" {
			matcher = styleImport
		}
		for _, match := range matcher.FindAllStringSubmatch(string(body), -1) {
			target := path.Clean(path.Join(path.Dir(name), match[1]))
			if !fs.ValidPath(target) {
				t.Errorf("%s references invalid asset path %q", name, match[1])
				continue
			}
			if _, err := fs.Stat(files, target); err != nil {
				t.Errorf("%s references missing embedded asset %q", name, target)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded browser files: %v", err)
	}
}

func TestIndexIsSelfContainedAndAccessible(t *testing.T) {
	t.Parallel()

	body, err := fs.ReadFile(Files(), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	for _, fragment := range []string{
		`<title>$OOSHDAQ · Collect, match, swap</title>`,
		`aria-label="$OOSHDAQ dashboard"`,
		`<strong>$OOSHDAQ</strong>`,
		`<h1 id="login-title">$OOSHDAQ</h1>`,
		`href="/styles.css"`,
		`href="/assets/ooshdaq-mark.svg"`,
		`<script src="/app.js" type="module"></script>`,
		`class="skip-link"`,
		`id="announcer"`,
		`aria-live="polite"`,
		`id="matches-nav-badge"`,
		`src="/assets/bean-buddies.svg"`,
		`<span>Find a collectible</span>`,
		`placeholder="Maui, Moana, or Grogu"`,
	} {
		if !strings.Contains(html, fragment) {
			t.Errorf("index.html is missing required fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"style=",
		"fonts.googleapis.com",
		"unpkg.com",
		"https://cdn",
		"www.woolworths.com",
		"<dialog",
		`id="collection-form"`,
		"Update quantities",
		`id="catalog-brand"`,
		`id="catalog-status"`,
		`id="match-filters"`,
		`id="match-give-catalog"`,
		`id="match-receive-catalog"`,
		`id="match-scope"`,
		"Swap leads",
		"Reciprocal scope",
		"reservation or promise",
		"trusted channel",
	} {
		if strings.Contains(html, forbidden) {
			t.Errorf("index.html contains forbidden inline or external content %q", forbidden)
		}
	}
}

func TestBrowserUsesOnlyBoundedProductContract(t *testing.T) {
	t.Parallel()

	files := Files()
	var browser strings.Builder
	err := fs.WalkDir(files, "js", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path.Ext(name) != ".js" {
			return nil
		}
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		browser.Write(body)
		browser.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	source := browser.String()
	for _, required := range []string{
		`activity: "/api/v1/activity"`,
		`catalogs: "/api/v1/catalogs"`,
		`summary: "/api/v1/collection/summary"`,
		"`/api/v1/catalogs/${encodeURIComponent(catalogID)}/collection`",
		"`/api/v1/collection/${encodeURIComponent(itemID)}`",
		`matches: "/api/v1/matches"`,
		`error.code === "invalid_cursor"`,
		`publication_status: "active"`,
		`collectionState`,
		`possible_swap_count`,
		`match.they_request === undefined`,
		`request.fulfillable !== (request.quantity > 0)`,
		`groupMatchesByWantedItem`,
		`unfulfillableMatches`,
		`"toggle-unfulfillable-matches"`,
		`"load-more-unfulfillable-matches"`,
		`Swaps you can’t fulfil yet`,
		`"aria-pressed": String(Boolean(entry.owned))`,
		`"1:0:1"`,
		`wanted: entry.wanted > 0 ? 1 : 0`,
		`owned: entry.owned > 0 ? 1 : 0`,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("browser source is missing bounded contract fragment %q", required)
		}
	}
	for _, forbidden := range []string{
		`"/api/v1/catalog"`,
		`"/api/v1/collection"`,
		"series_id",
		"series_name",
		"/api/v1/brands",
		"wanted cannot be combined with owned or available",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("browser source contains removed contract surface %q", forbidden)
		}
	}
}

func TestAuthenticatedBootstrapIsLazy(t *testing.T) {
	t.Parallel()

	body, err := fs.ReadFile(Files(), "js/controller.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	start := strings.Index(source, "export async function loadSession()")
	end := strings.Index(source[start:], "export function invalidateAfterWrite()")
	if start < 0 || end < 0 {
		t.Fatal("could not isolate loadSession")
	}
	loadSession := source[start : start+end]
	if !strings.Contains(loadSession, "loadCatalogs()") ||
		!strings.Contains(loadSession, "loadOverallSummary()") {
		t.Fatal("authenticated bootstrap must load bounded catalogs and the overall summary")
	}
	if !strings.Contains(loadSession, "void recordActivity()") ||
		strings.Contains(loadSession, "await recordActivity()") {
		t.Fatal("authenticated bootstrap must record activity without blocking rendering")
	}
	if strings.Contains(loadSession, "loadCollection()") ||
		strings.Contains(loadSession, "loadMatches()") ||
		strings.Contains(loadSession, "loadCatalogDirectory()") {
		t.Fatal("authenticated bootstrap eagerly loads route-scoped resources")
	}
}

func TestProxyModeHidesLocalLogoutControl(t *testing.T) {
	t.Parallel()

	body, err := fs.ReadFile(Files(), "js/render-shell.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		string(body),
		`elements.logoutButton.hidden = state.session.authMode !== "mock";`,
	) {
		t.Error("proxy mode must hide the local-only logout control")
	}
}
