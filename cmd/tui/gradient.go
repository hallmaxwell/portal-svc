package tui

import (
	"fmt"
	"strings"

	"github.com/lucasb-eyer/go-colorful"
)

type HexColor string

func gradientRender(lines []string, startColor HexColor, endColor HexColor) ([]string, error) {
	c1, err := colorful.Hex(string(startColor))
	if err != nil {
		return nil, err
	}
	c2, err := colorful.Hex(string(endColor))
	if err != nil {
		return nil, err
	}

	maxWidth := 0
	for _, line := range lines {
		runes := []rune(line)
		if len(runes) > maxWidth {
			maxWidth = len(runes)
		}
	}

	var renderedLines []string
	for _, line := range lines {
		runes := []rune(line)
		var b strings.Builder
		for i, r := range runes {
			if r == ' ' {
				b.WriteRune(r)
				continue
			}
			t := 0.0
			if maxWidth > 1 {
				t = float64(i) / float64(maxWidth-1)
			}
			c := c1.BlendRgb(c2, t)
			r8, g8, b8 := c.RGB255()

			if r == '▒' {
				// Muted border color for shadow using semantic theme border
				// Using lipgloss equivalent truecolor: 42,42,42
				b.WriteString("\x1b[38;2;42;42;42m▒\x1b[0m")
			} else {
				b.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm%c\x1b[0m", r8, g8, b8, r))
			}
		}
		renderedLines = append(renderedLines, b.String())
	}
	return renderedLines, nil
}
