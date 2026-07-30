package main

import (
	"fmt"
	"io"
	"strings"
)

const (
	helpLineWidth         = 88
	helpDescriptionColumn = 34
)

type helpDocument struct {
	Description string
	Usage       string
	Commands    []helpCommand
	Groups      []helpOptionGroup
	Examples    []string
	Footer      string
}

type helpCommand struct {
	Name    string
	Summary string
}

type helpOptionGroup struct {
	Title   string
	Options []helpOption
}

type helpOption struct {
	Name         string
	Short        string
	Metavariable string
	Summary      string
	Repeatable   bool
	Available    func() []string
}

func renderHelp(output io.Writer, document helpDocument) error {
	var rendered strings.Builder
	writeHelpParagraph(&rendered, document.Description)
	writeHelpSection(&rendered, "Usage", []string{"  " + document.Usage})
	if len(document.Commands) > 0 {
		writeCommandSection(&rendered, document.Commands)
	}
	for _, group := range document.Groups {
		if err := writeOptionSection(&rendered, group); err != nil {
			return err
		}
	}
	if len(document.Examples) > 0 {
		lines := make([]string, 0, len(document.Examples))
		for _, example := range document.Examples {
			lines = append(lines, indentLines(example, "  ")...)
		}
		writeHelpSection(&rendered, "Examples", lines)
	}
	writeHelpParagraph(&rendered, document.Footer)
	_, err := io.WriteString(output, rendered.String())
	return err
}

func writeHelpParagraph(output *strings.Builder, value string) {
	if value == "" {
		return
	}
	for _, line := range wrapText(value, helpLineWidth) {
		output.WriteString(line)
		output.WriteByte('\n')
	}
	output.WriteByte('\n')
}

func writeHelpSection(output *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	output.WriteString(title)
	output.WriteString(":\n")
	for _, line := range lines {
		output.WriteString(line)
		output.WriteByte('\n')
	}
	output.WriteByte('\n')
}

func writeCommandSection(output *strings.Builder, commands []helpCommand) {
	width := 0
	for _, command := range commands {
		if len(command.Name) > width {
			width = len(command.Name)
		}
	}
	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		lines = append(lines, fmt.Sprintf("  %-*s  %s", width, command.Name, command.Summary))
	}
	writeHelpSection(output, "Commands", lines)
}

func writeOptionSection(output *strings.Builder, group helpOptionGroup) error {
	if len(group.Options) == 0 {
		return nil
	}
	output.WriteString(group.Title)
	output.WriteString(":\n")
	for _, option := range group.Options {
		summary := option.Summary
		if option.Available != nil {
			available := option.Available()
			if len(available) == 0 {
				return fmt.Errorf("help option --%s has no available values", option.Name)
			}
			summary += " (available: " + strings.Join(available, ", ") + ")"
		}
		if option.Repeatable {
			summary += " (repeatable)"
		}
		writeOption(output, optionLabel(option), summary)
	}
	output.WriteByte('\n')
	return nil
}

func optionLabel(option helpOption) string {
	name := "--" + option.Name
	if option.Short != "" {
		name = "-" + option.Short + ", " + name
	}
	if option.Metavariable != "" {
		name += " <" + option.Metavariable + ">"
	}
	return name
}

func writeOption(output *strings.Builder, label, summary string) {
	label = "  " + label
	indent := strings.Repeat(" ", helpDescriptionColumn)
	width := helpLineWidth - helpDescriptionColumn
	lines := wrapText(summary, width)
	if len(label) >= helpDescriptionColumn-1 {
		output.WriteString(label)
		output.WriteByte('\n')
		output.WriteString(indent)
		output.WriteString(lines[0])
		output.WriteByte('\n')
	} else {
		output.WriteString(label)
		output.WriteString(strings.Repeat(" ", helpDescriptionColumn-len(label)))
		output.WriteString(lines[0])
		output.WriteByte('\n')
	}
	for _, line := range lines[1:] {
		output.WriteString(indent)
		output.WriteString(line)
		output.WriteByte('\n')
	}
}

func wrapText(value string, width int) []string {
	words := strings.Fields(value)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{words[0]}
	for _, word := range words[1:] {
		last := len(lines) - 1
		if len(lines[last])+1+len(word) <= width {
			lines[last] += " " + word
		} else {
			lines = append(lines, word)
		}
	}
	return lines
}

func indentLines(value, indent string) []string {
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = indent + lines[index]
	}
	return lines
}
