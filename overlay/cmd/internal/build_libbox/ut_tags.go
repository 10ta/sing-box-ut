package main

// ut: adds the XHTTP build tag to every libbox (Android/Apple) build without
// editing upstream's tag list in main.go, which changes often.
func init() {
	sharedTags = append(sharedTags, "with_xhttp")
}
