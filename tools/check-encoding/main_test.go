package main

import "testing"

func TestFindMojibake(t *testing.T) {
	t.Parallel()

	text := "name: \u0420\u0454\u0420\u00b0\u0421\u0402\u0421\u201a\u0420\u0455\u0421\u2021\u0420\u0454\u0420\u0451"
	line, sample := findMojibake(text)
	if line != 1 {
		t.Fatalf("expected mojibake on line 1, got line %d", line)
	}
	if sample == "" {
		t.Fatal("expected sample")
	}
}

func TestFindMojibakeIgnoresNormalRussianText(t *testing.T) {
	t.Parallel()

	line, sample := findMojibake("Карточки товаров для маркетплейсов")
	if line != 0 || sample != "" {
		t.Fatalf("expected no mojibake, got line %d and sample %q", line, sample)
	}
}

func TestFindMojibakeReturnsLineNumber(t *testing.T) {
	t.Parallel()

	text := "ok\nvalue: \u0420\u0459\u0420\u00b0\u0421\u0402\u0421\u201a\u0420\u0455\u0421\u2021\u0420\u0454\u0420\u0451"
	line, _ := findMojibake(text)
	if line != 2 {
		t.Fatalf("expected mojibake on line 2, got line %d", line)
	}
}
