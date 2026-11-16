package main

import (
	_ "embed"
	"encoding/json"
	"sync/atomic"
)

//go:embed translations.en.json
var englishCatalogJSON []byte
var englishCatalog = func() map[string]string {
	var catalog map[string]string
	if err := json.Unmarshal(englishCatalogJSON, &catalog); err != nil {
		panic(err)
	}
	return catalog
}()
var activeEnglish atomic.Bool

// Translate only application-owned strings at their construction sites. Remote
// content, user input, model replies and plugin data never pass through this map.
func tr(source string) string {
	if activeEnglish.Load() {
		if translated, ok := englishCatalog[source]; ok {
			return translated
		}
	}
	return source
}
