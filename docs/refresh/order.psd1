# The Refresh order, as docs/refresh/dependency-order.md declares it. Invoke-Build Refresh reads this file and nothing
# else for its steps; Pester fails when it and the doc's sequence table disagree, or when it breaks a rule R1 to R6.
#   Id              unique step id, the doc's Step column
#   Repo            Portal or Portal.<Name>, a folder beside this repo (Portal is this repo)
#   Action          invoke (Invoke-Build Task -File Repo\File in a child pwsh), approve (POST .../snap) or catalogue
#   Phase           down, up or check
#   NeedsPortal     the step talks to the portal on 8200
#   RestartsPortal  the step stops or replaces the portal (drops every unapproved layer)
#   PushesLive      the step mounts layers that only an approval after it keeps
#   SkipWithoutDocker  skip the step, saying so, when docker info gets no engine
@{
    Steps    = @(
        @{ Id = 'agent-down'; Repo = 'Portal.Agent'; Action = 'invoke'; File = 'Portal.Agent.build.ps1'; Task = 'Down'; Phase = 'down'; NeedsPortal = $true; RestartsPortal = $false; PushesLive = $false; SkipWithoutDocker = $true }
        @{ Id = 'portal-down'; Repo = 'Portal'; Action = 'invoke'; File = '.build.ps1'; Task = 'Down'; Phase = 'down'; NeedsPortal = $false; RestartsPortal = $true; PushesLive = $false; SkipWithoutDocker = $false }
        @{ Id = 'portal-dev'; Repo = 'Portal'; Action = 'invoke'; File = '.build.ps1'; Task = 'Dev'; Phase = 'up'; NeedsPortal = $false; RestartsPortal = $true; PushesLive = $false; SkipWithoutDocker = $false }
        @{ Id = 'emitters-mount'; Repo = 'Portal.Emitters'; Action = 'invoke'; File = 'Portal.Emitters.build.ps1'; Task = 'Mount'; Phase = 'up'; NeedsPortal = $true; RestartsPortal = $true; PushesLive = $false; SkipWithoutDocker = $false }
        @{ Id = 'agent-up'; Repo = 'Portal.Agent'; Action = 'invoke'; File = 'Portal.Agent.build.ps1'; Task = 'Up'; Phase = 'up'; NeedsPortal = $true; RestartsPortal = $false; PushesLive = $true; SkipWithoutDocker = $false }
        @{ Id = 'approve'; Repo = 'Portal'; Action = 'approve'; Phase = 'up'; NeedsPortal = $true; RestartsPortal = $false; PushesLive = $false; SkipWithoutDocker = $false }
        @{ Id = 'catalogue'; Repo = 'Portal'; Action = 'catalogue'; Phase = 'check'; NeedsPortal = $true; RestartsPortal = $false; PushesLive = $false; SkipWithoutDocker = $false }
    )
    # Repos with no build script and so no step: Refresh prints one line for each.
    CardOnly = @('Portal.Ontology', 'Portal.Skills', 'Portal.Tools', 'Portal.Workflows')
}
