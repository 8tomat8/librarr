package sources

// SwapDefaultRegistryURL replaces the built-in default URL with newURL and
// returns a function that restores the previous value. Test-only.
func SwapDefaultRegistryURL(newURL string) func() {
	prev := defaultRegistryURL
	defaultRegistryURL = newURL
	return func() { defaultRegistryURL = prev }
}
