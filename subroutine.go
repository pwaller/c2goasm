package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var regexpRet = regexp.MustCompile(`^\s*ret`)

type Subroutine struct {
	name     string
	body     []string
	epilogue Epilogue
}

type Global struct {
	dotGlobalLine   int
	globalName      string
	globalLabelLine int
}

func splitOnGlobals(lines []string) []Global {

	var result []Global

	for index, line := range lines {
		if strings.Contains(line, ".globl") {

			scrambled := strings.TrimSpace(strings.Split(line, ".globl")[1])
			name := extractName(scrambled)

			labelLine := findLabel(lines, scrambled)

			result = append(result, Global{dotGlobalLine: index, globalName: name, globalLabelLine: labelLine})
		}
	}

	return result
}

// Segment the source into multiple routines
func segmentSource(src []string) []Subroutine {

	globals := splitOnGlobals(src)

	if len(globals) == 0 {
		return []Subroutine{}
	}

	subroutines := []Subroutine{}

	splitBegin := globals[0].dotGlobalLine
	for iglobal, global := range globals {
		splitEnd := len(src)
		if iglobal < len(globals)-1 {
			splitEnd = globals[iglobal+1].dotGlobalLine
		}

		// Search for `ret` statement
		for lineRet := splitBegin; lineRet < splitEnd; lineRet++ {
			if match := regexpRet.FindStringSubmatch(src[lineRet]); len(match) > 0 {

				newsub := extractSubroutine(lineRet, src, global)

				subroutines = append(subroutines, newsub)
				
				break
			}
		}

		splitBegin = splitEnd
	}

	return subroutines
}

var disabledForTesting = false

func extractSubroutine(lineRet int, src []string, global Global) Subroutine {

	bodyStart := global.globalLabelLine + 1
	bodyEnd := lineRet + 1

	// loop until all missing labels are found
	for !disabledForTesting {
		missingLabels := getMissingLabels(src[bodyStart:bodyEnd])

		if len(missingLabels) == 0 {
			break
		}

		// add the missing lines in order to find the missing labels
		postEpilogueLines := getMissingLines(src, bodyEnd-1, missingLabels)

		bodyEnd += postEpilogueLines
	}


	subroutine := Subroutine{
		name: global.globalName,
		body: src[bodyStart:bodyEnd],
		epilogue: extractEpilogue(src[bodyStart:bodyEnd]),
	}

	// Remove prologue lines from subroutine
	subroutine.removePrologueLines(src, bodyStart, bodyEnd)

	return subroutine
}

func (s *Subroutine) removePrologueLines(src []string, bodyStart int, bodyEnd int)  {

	prologueLines := getPrologueLines(src[bodyStart:bodyEnd], &s.epilogue)

	// Remove prologue lines from body
	s.body = s.body[prologueLines:]

	// Adjust range of epilogue accordingly
	s.epilogue.Start -= prologueLines
	s.epilogue.End -= prologueLines
}

func extractEpilogue(src []string) Epilogue {

	for iline, line := range src {

		if match := regexpRet.FindStringSubmatch(line); len(match) > 0 {

			// Found closing ret statement, start searching back to first non epilogue instruction
			epilogueStart := iline
			for ; epilogueStart >= 0; epilogueStart-- {
				if !isEpilogueInstruction(src[epilogueStart]) {
					epilogueStart++
					break
				}
			}

			epilogue := extractEpilogueInfo(src, epilogueStart, iline+1)

			return epilogue
		}
	}

	panic("Failed to find 'ret' instruction")
}

func getMissingLabels(src []string) map[string]bool {

	labelMap := make(map[string]bool)
	jumpMap := make(map[string]bool)

	for _, line := range src {

		line, _ := stripComments(line)
		if _, label := fixLabels(line); label != "" {
			labelMap[label] = true
		}
		if _, _, label := upperCaseJumps(line); label != "" {
			jumpMap[label] = true
		}

	}

	for label, _ := range labelMap {
		if _, ok := jumpMap[label]; ok {
			delete(jumpMap, label)
		} else {
			panic("label not found")
		}
	}

	return jumpMap
}

func getMissingLines(src []string, lineRet int, missingLabels map[string]bool) int {

	var iline int
	// first scan until we've found the missing labels
	for iline = lineRet; len(missingLabels) > 0 && iline < len(src); iline++ {
		line := src[iline]
		_, label := fixLabels(line)
		if label != "" {
			if _, ok := missingLabels[label]; ok {
				delete(missingLabels, label)
			}
		}
	}
	// then scan until we find an (unconditional) JMP
	for ; iline < len(src); iline++ {
		line := src[iline]
		_, jump, _ := upperCaseJumps(line)
		if jump == "JMP" {
			break
		}
	}

	return iline - lineRet
}

func getPrologueLines(lines []string, epilogue *Epilogue) int {

	index, line := 0, ""

	for index, line = range lines {

		var skip bool
		line, skip = stripComments(line) // Remove ## comments
		if skip {
			continue
		}

		if !epilogue.IsPrologueInstruction(line) {
			break
		}
	}

	return index
}

func findLabel(lines []string, label string) int {

	labelDef := label + ":"

	for index, line := range lines {
		if strings.HasPrefix(line, labelDef) {
			return index
		}
	}

	panic(fmt.Sprintf("Failed to find label: %s", labelDef))
}

func extractNamePart(part string) (int, string) {

	digits := 0
	for _, d := range part {
		if unicode.IsDigit(d) {
			digits += 1
		} else {
			break
		}
	}
	length, _ := strconv.Atoi(part[:digits])
	return digits + length, part[digits:(digits + length)]
}

func extractName(name string) string {

	var parts []string

	for index, ch := range name {
		if unicode.IsDigit(ch) {

			for index < len(name) {
				size, part := extractNamePart(name[index:])
				if size == 0 {
					break
				}

				parts = append(parts, part)
				index += size
			}

			break
		}
	}

	return strings.Join(parts, "")
}
