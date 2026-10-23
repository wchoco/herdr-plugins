package herdr

import "time"

// SetCommandTimeout shortens a Command's timeout for tests.
func SetCommandTimeout(c *Command, d time.Duration) { c.timeout = d }
