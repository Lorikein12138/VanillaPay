package payments

import "crypto/rand"

// Passwords are generated per test process and are never stored in the repository.
var (
	fixturePassword = "Aa1!" + rand.Text()
)
