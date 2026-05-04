package components

const LongAsciiLogo = `

_|_|_|      _|_|    _|_|_|    _|_|_|_|_|    _|_|    _|
_|    _|  _|    _|  _|    _|      _|      _|    _|  _|
_|_|_|    _|    _|  _|_|_|        _|      _|_|_|_|  _|
_|        _|    _|  _|    _|      _|      _|    _|  _|
_|          _|_|    _|    _|      _|      _|    _|  _|_|_|_|

`

const ShortAsciiLogo = `
    ____  ____  ____  _________    __
   / __ \/ __ \/ __ \/_  __/   |  / /
  / /_/ / / / / /_/ / / / / /| | / /
 / ____/ /_/ / _, _/ / / / ___ |/ /___
/_/    \____/_/ |_| /_/ /_/  |_/_____/

`

const TinyAsciiLogo = `
 _  _  ____
|_)/ \|_)| /\ |
|  \_/| \|/--\|_

`

// GetLogoByWidth returns the appropriate ASCII art logo based on the provided width.
func GetLogoByWidth(width int) string {
	if width >= 65 {
		return LongAsciiLogo
	} else if width >= 40 {
		return ShortAsciiLogo
	}
	return TinyAsciiLogo
}
