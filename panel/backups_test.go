package main

import (
	"strings"
	"testing"
	"time"
)

func TestAgeLevel(t *testing.T) {
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{1 * time.Hour, "ok"},
		{23 * time.Hour, "ok"},
		{25 * time.Hour, "warn"},
		{71 * time.Hour, "warn"},
		{73 * time.Hour, "crit"},
		{200 * time.Hour, "crit"},
	}
	for _, c := range cases {
		got := ageLevel(time.Now().Add(-c.ago))
		if got != c.want {
			t.Errorf("ageLevel(hace %v) = %q, want %q", c.ago, got, c.want)
		}
	}
}

func TestHumanAgo(t *testing.T) {
	cases := []struct {
		ago      time.Duration
		contains string
	}{
		{30 * time.Second, "instantes"},
		{5 * time.Minute, "min"},
		{3 * time.Hour, "h"},
		{48 * time.Hour, "días"},
	}
	for _, c := range cases {
		got := humanAgo(time.Now().Add(-c.ago))
		if !strings.Contains(got, c.contains) {
			t.Errorf("humanAgo(hace %v) = %q, esperaba que contuviera %q", c.ago, got, c.contains)
		}
	}
}
