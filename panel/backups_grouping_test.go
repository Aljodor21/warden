package main

import "testing"

func TestGroupBackupRuns(t *testing.T) {
	snaps := []Snapshot{
		{ID: "old1", Time: "2026-06-23T00:32:50Z", Tags: []string{"files"}, WhenFull: "23 Jun"},
		{ID: "old2", Time: "2026-06-23T00:32:52Z", Tags: []string{"db"}, WhenFull: "23 Jun"},
		{ID: "new1", Time: "2026-06-24T15:00:34Z", Tags: []string{"files"}, WhenFull: "24 Jun"},
		{ID: "new2", Time: "2026-06-24T15:00:35Z", Tags: []string{"db"}, WhenFull: "24 Jun"},
	}
	snaps[0].Summary.TotalBytesProcessed = 36_000_000 // viejo, con contenido real
	snaps[2].Summary.TotalBytesProcessed = 78         // nuevo, casi vacío

	runs := groupBackupRuns(snaps)
	if len(runs) != 2 {
		t.Fatalf("esperaba 2 corridas, hubo %d", len(runs))
	}
	// Más reciente primero.
	if runs[0].FilesID != "new1" || runs[1].FilesID != "old1" {
		t.Fatalf("orden incorrecto: %+v", runs)
	}
	// Cada files debe emparejarse con su db más cercano en el tiempo.
	if runs[0].DBID != "new2" {
		t.Errorf("new1 debería emparejar con new2, empareja con %q", runs[0].DBID)
	}
	if runs[1].DBID != "old2" {
		t.Errorf("old1 debería emparejar con old2, empareja con %q", runs[1].DBID)
	}
	// El tamaño confirma cuál corrida tiene contenido real.
	if runs[1].FilesSize == "" {
		t.Error("la corrida vieja con 36MB debería mostrar tamaño")
	}
}
