package main

import (
	"context"
	"os/exec"
	"time"
)

// bgCtx10min: tope de seguridad para procesos que esperan algo del usuario
// (ej. completar un login en el navegador) — si nadie lo termina, no queda
// corriendo para siempre.
func bgCtx10min() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Minute)
}

// bgCtx3min: tope para procesos que NO esperan a un humano, solo pueden
// tardar por red/descarga (ej. registrar un runner).
func bgCtx3min() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Minute)
}

// runInBackground corre un comando volcando su stdout/stderr en vivo al
// bgProcess (para que el panel lo muestre con polling), y marca 'done'
// al terminar. cmd.Stdin queda en nil (sin TTY) a propósito.
func runInBackground(ctx context.Context, proc *bgProcess, name string, args ...string) {
	defer proc.finish()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = nil
	cmd.Stdout = proc
	cmd.Stderr = proc
	_ = cmd.Run()
}
