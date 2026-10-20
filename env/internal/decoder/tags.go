package decoder

import (
	"reflect"
	"strconv"
	"strings"
)

type fieldTags struct {
	env, binding, fallback, separator, prefix string
	hasEnv, hasBinding, hasFallback           bool
	hasSeparator, hasPrefix                   bool
}

func readTags(tag reflect.StructTag) (fieldTags, bool) {
	var tags fieldTags
	tags.env, tags.hasEnv = tag.Lookup("env")
	tags.binding, tags.hasBinding = tag.Lookup("binding")
	tags.fallback, tags.hasFallback = tag.Lookup("fallback")
	tags.separator, tags.hasSeparator = tag.Lookup("separator")
	tags.prefix, tags.hasPrefix = tag.Lookup("prefix")
	return tags, validOwnedTags(string(tag))
}

// validOwnedTags follows StructTag's key/quoted-value scanning rules, but rejects
// malformed owned entries instead of letting Lookup treat them as absent.
// Unrelated entries are skipped without interpreting their values.
func validOwnedTags(tag string) bool {
	for tag != "" {
		for len(tag) > 0 && tag[0] == ' ' {
			tag = tag[1:]
		}
		if tag == "" {
			break
		}
		keyEnd := 0
		for keyEnd < len(tag) && tag[keyEnd] > ' ' && tag[keyEnd] != ':' && tag[keyEnd] != '"' && tag[keyEnd] != 0x7f {
			keyEnd++
		}
		owned := ownedTag(tag[:keyEnd])
		if keyEnd == 0 || keyEnd+1 >= len(tag) || tag[keyEnd] != ':' || tag[keyEnd+1] != '"' {
			// Lookup also stops at an unrelated malformed entry. Do not add
			// validation requirements for another package's tags.
			return !owned
		}
		tag = tag[keyEnd+1:]
		quoteEnd := 1
		for quoteEnd < len(tag) && tag[quoteEnd] != '"' {
			if tag[quoteEnd] == '\\' {
				quoteEnd++
			}
			quoteEnd++
		}
		if quoteEnd >= len(tag) {
			return !owned
		}
		if owned {
			if _, err := strconv.Unquote(tag[:quoteEnd+1]); err != nil {
				return false
			}
		}
		tag = tag[quoteEnd+1:]
	}
	return true
}

func ownedTag(key string) bool {
	switch key {
	case "env", "binding", "fallback", "separator", "prefix":
		return true
	default:
		return false
	}
}

type bindingOptions struct {
	required bool
	nonempty bool
	notrim   bool
}

func parseBinding(tags fieldTags) (bindingOptions, bool) {
	var options bindingOptions
	if !tags.hasBinding {
		return options, true
	}
	seen := make(map[string]bool)
	for _, flag := range strings.Split(tags.binding, ",") {
		flag = strings.TrimSpace(flag)
		if seen[flag] {
			return options, false
		}
		seen[flag] = true
		switch flag {
		case "optional":
		case "required":
			options.required = true
		case "nonempty":
			options.nonempty = true
		case "notrim":
			options.notrim = true
		default:
			return options, false
		}
	}
	return options, !(options.required && (seen["optional"] || tags.hasFallback))
}
