package keys

// Codes of keys that only Canon produces.
const (
	Tab   rune = 9
	Space rune = ' '
)

// Canon returns k's canonical form: keys a legacy terminal sends as the
// same bytes get the same form, so a key bound by name matches however the
// terminal encoded it. Ctrl-M is Enter, Ctrl-I is Tab, the NUL byte is
// Ctrl-Space, and Shift on a lone letter is the uppercase letter. Other
// modifiers, Alt included, are kept.
func Canon(k Key) Key {
	switch {
	case k.Mod&Ctrl != 0 && k.Code == 'm':
		return Key{Code: Enter, Mod: k.Mod &^ Ctrl}
	case k.Mod&Ctrl != 0 && k.Code == 'i':
		return Key{Code: Tab, Mod: k.Mod &^ Ctrl}
	case k.Mod&Ctrl != 0 && k.Code == '`': // NUL, decoded as Ctrl + 0x40 + 'a'-'A'
		return Key{Code: Space, Mod: k.Mod}
	case k.Mod&(Shift|Ctrl) == Shift && k.Code >= 'a' && k.Code <= 'z':
		return Key{Code: k.Code - 'a' + 'A', Mod: k.Mod &^ Shift}
	}
	return k
}
