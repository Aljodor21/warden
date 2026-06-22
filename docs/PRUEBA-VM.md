# Probar warden en una máquina virtual

Objetivo: instalar warden en un **Ubuntu Server limpio** (VM) sin tocar tu server
real, y debuguear juntos.

## 1. Crear la VM (CachyOS / Arch, con virt-manager)

```bash
sudo pacman -S --needed virt-manager qemu-full dnsmasq
sudo systemctl enable --now libvirtd
```

Abrí **virt-manager** → *New VM* → ISO de **Ubuntu Server 24.04** →
2 GB RAM, 2 vCPU, 20 GB disco → instalá Ubuntu (marcá *Install OpenSSH server*).

## 2. Entrar a la VM

Averiguá su IP (en virt-manager, o `ip a` dentro de la VM) y entrá por SSH:

```bash
ssh usuario@IP-DE-LA-VM
```

## 3. Meter warden en la VM

El repo es privado; para debuguear lo más cómodo es **copiar la carpeta** desde
tu PC (sin tus secretos ni el `.git`):

```bash
# en tu PC (CachyOS):
rsync -av --exclude site/ --exclude .git ~/proyectos/warden/ usuario@IP-DE-LA-VM:~/warden/
```

(Alternativa: cloná con un token → `git clone https://TOKEN@github.com/Aljodor21/warden.git`.)

## 4. Configurar el sitio (en la VM)

```bash
cd ~/warden
mkdir -p site/catalog
cp examples/site.conf.example site/site.conf
nano site/site.conf      # WARDEN_HOSTNAME, WARDEN_LAN, WARDEN_TIMEZONE
```

## 5. Instalar

```bash
sudo ./bootstrap.sh      # elegí el preset 'básico' para la primera prueba
```

## 6. Verificar

```bash
warden doctor
```

Y abrí en tu navegador (IP de la VM):
- Homepage → `http://IP:7575`
- Cockpit  → `https://IP:9090`

## 7. (Opcional) Probar la restauración

Para restaurar, la VM necesita ver el disco de backup:
- virt-manager → *Add Hardware* → *USB Host Device* → tu disco (passthrough), o
- creá un disco virtual y copiale el repositorio restic.

Luego: `sudo warden restore`.

## Debug — errores comunes

| Síntoma | Qué revisar |
|---|---|
| "Distro no soportada" | La VM debe ser Debian/Ubuntu o Arch |
| gum no instala | Sigue en modo texto plano (funciona igual) |
| Homepage no carga | `HOMEPAGE_ALLOWED_HOSTS` (probá `*`) |
| docker sin permisos | Cerrá sesión y volvé a entrar (grupo docker) |
| Ver qué haría sin tocar nada | `WARDEN_DRY_RUN=1 sudo ./bootstrap.sh` |

Anotá cualquier error y lo resolvemos juntos.
