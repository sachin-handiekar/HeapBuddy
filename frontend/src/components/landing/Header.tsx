import { Link } from "@tanstack/react-router";
import { Github, Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTheme } from "@/lib/theme";

const GITHUB_URL = "https://github.com/sachin-handiekar/heapbuddy";

export function Header() {
  const { theme, toggle } = useTheme();

  return (
    <header className="sticky top-0 z-50 border-b border-border/60 bg-background/80 backdrop-blur-xl">
      <div className="mx-auto flex h-14 max-w-7xl items-center justify-between px-4 sm:px-6">
        <Link to="/" className="flex items-center gap-2">
          <Logo />
          <span className="font-semibold tracking-tight">HeapBuddy</span>
          <span className="hidden rounded-md border border-border bg-muted/40 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground sm:inline">
            v0.1
          </span>
        </Link>

        <nav className="hidden items-center gap-7 text-sm text-muted-foreground md:flex">
          <a href="#features" className="transition-colors hover:text-foreground">Features</a>
          <a href="#how" className="transition-colors hover:text-foreground">How it works</a>
          <a href="#faq" className="transition-colors hover:text-foreground">FAQ</a>
          <a href={GITHUB_URL} target="_blank" rel="noreferrer" className="transition-colors hover:text-foreground">Docs</a>
        </nav>

        <div className="flex items-center gap-2">
          <Button asChild variant="outline" size="sm" className="hidden gap-2 sm:inline-flex">
            <a href={GITHUB_URL} target="_blank" rel="noreferrer">
              <Github className="h-3.5 w-3.5" />
              <span>GitHub</span>
            </a>
          </Button>
          <Button variant="ghost" size="icon" onClick={toggle} aria-label="Toggle theme" className="h-8 w-8">
            {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </Button>
          <Button asChild size="sm" className="gap-1.5">
            <Link to="/analyze">Analyze a heap dump</Link>
          </Button>
        </div>
      </div>
    </header>
  );
}

function Logo() {
  return (
    <div className="relative grid h-7 w-7 place-items-center rounded-md bg-gradient-to-br from-[color:var(--accent-violet)] to-[color:var(--accent-indigo)] text-primary-foreground shadow-[inset_0_1px_0_rgba(255,255,255,0.2)]">
      <svg viewBox="0 0 20 20" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="2">
        <circle cx="9" cy="9" r="5" />
        <path d="M13 13l4 4" strokeLinecap="round" />
      </svg>
    </div>
  );
}
