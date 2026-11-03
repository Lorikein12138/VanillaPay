package settings

import "crypto/rand"

// Passwords are generated per test process and are never stored in the repository.
var (
	fixtureChangedPassword = "Aa1!" + rand.Text()
	fixturePassword        = "Aa1!" + rand.Text()
	fixtureWrongPassword   = "Aa1!" + rand.Text()
)
