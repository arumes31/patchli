# Patchli - Distributed Linux Patch Management & Fleet Monitoring System

Patchli is a high-performance, distributed Patch Management system designed to seamlessly orchestrate updates across massive fleets of Linux (and Windows) nodes.

## Architecture

*   **Control Plane:** Go 1.26, PostgreSQL 15, Redis 7. Features WebSockets and gRPC for real-time communication, and a Worker Pool for job orchestration.
*   **Intelligent Agent:** Go 1.26. A lightweight, self-updating, self-healing agent that supports native package managers.

## Advanced Features Implemented

*   **OS Support:** Debian/Ubuntu (APT), Alpine (APK), RHEL/CentOS/Rocky (DNF/YUM), Arch Linux (Pacman), SUSE (Zypper), and Windows Server (WUA).
*   **Resilience:** Self-healing watchdog, HTTP long-polling fallback, persistent UUID identity, state recovery after reboots.
*   **Orchestration:** Maintenance Windows, Pre/Post-Patch Scripts, Health Checks, Job Pausing/Resuming.
*   **Security:** HMAC-based zero-touch registration, JWT persistent authentication, Disk Space & Process Lock pre-checks.
*   **CI/CD:** Automated testing, Code Coverage enforcement (80%), Chaos Engineering tests (Pumba), `gosec` SAST, and `govulncheck`.

## Getting Started

1.  Start the Control Plane: `docker-compose up -d`
2.  Navigate to the dashboard at `http://localhost:8080/`
3.  Click **Add Agent** to generate a secure installation script.
