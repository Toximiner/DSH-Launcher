// Browser paste events are forwarded directly to the shared TUI model.
package clipboard

import "errors"

func ReadAll() (string, error) { return "", errors.New("use the browser paste action") }
func WriteAll(string) error    { return errors.New("use browser text selection") }
