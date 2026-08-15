"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  ChevronsUpDown,
  Cpu,
  KeyRound,
  LayoutGrid,
  LifeBuoy,
  LogOut,
  ScrollText,
  Settings,
  Users,
} from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";
import { Avatar } from "@/components/ui/avatar";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";
import type { SessionUser } from "@/lib/session-user";

type NavItem = {
  href: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
};

/**
 * Organization-level navigation.
 *
 * Grouped by the question each section answers, rather than by object type:
 * what exists, who can reach it, and what happened. Project-scoped navigation
 * lives inside a project, so this bar keeps the same shape everywhere.
 */
const PLATFORM: NavItem[] = [
  { href: "/projects", label: "Projects", icon: LayoutGrid },
  { href: "/identities", label: "Identities", icon: Cpu },
];

const GOVERNANCE: NavItem[] = [
  { href: "/audit", label: "Audit", icon: ScrollText },
  { href: "/members", label: "Members", icon: Users },
];

export function AppSidebar({ user }: { user: SessionUser }) {
  const pathname = usePathname();

  const isActive = (href: string) =>
    pathname === href || pathname.startsWith(`${href}/`);

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <OrganizationSwitcher user={user} />
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Platform</SidebarGroupLabel>
          <SidebarMenu>
            {PLATFORM.map((item) => (
              <NavEntry
                key={item.href}
                item={item}
                active={isActive(item.href)}
              />
            ))}
          </SidebarMenu>
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupLabel>Governance</SidebarGroupLabel>
          <SidebarMenu>
            {GOVERNANCE.map((item) => (
              <NavEntry
                key={item.href}
                item={item}
                active={isActive(item.href)}
              />
            ))}
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild tooltip="Documentation">
              {/*
                Points at this application's own documentation now that it has
                some. It used to open github.com, which was a placeholder.
              */}
              <Link href="/docs">
                <LifeBuoy />
                <span>Docs</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              isActive={isActive("/settings")}
              tooltip="Settings"
            >
              <Link href="/settings">
                <Settings />
                <span>Settings</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>

        <UserMenu user={user} />
      </SidebarFooter>

      <SidebarRail />
    </Sidebar>
  );
}

function NavEntry({ item, active }: { item: NavItem; active: boolean }) {
  const Icon = item.icon;
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={active} tooltip={item.label}>
        <Link href={item.href}>
          <Icon />
          <span>{item.label}</span>
        </Link>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

/**
 * The organization switcher.
 *
 * One organization per user in this build, so this is mostly an identity
 * marker, but it occupies the position people expect a switcher to be in, and
 * the plan badge carries the same information Vercel's does.
 */
function OrganizationSwitcher({ user }: { user: SessionUser }) {
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton
          size="lg"
          className="data-[state=open]:bg-sidebar-accent"
          tooltip={user.organization}
        >
          <div className="grid flex-1 text-left leading-tight">
            <span className="truncate text-meta font-medium">
              {user.organization}
            </span>
            <span className="truncate text-meta text-sidebar-foreground/55">
              Organization
            </span>
          </div>
          <ChevronsUpDown className="ml-auto size-3.5 text-sidebar-foreground/45" />
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}

function UserMenu({ user }: { user: SessionUser }) {
  const toast = useToast();

  async function signOut() {
    try {
      await apiFetch("/api/auth/logout", { method: "POST" });
    } catch {
      // The redirect happens regardless; a failed revoke is reported by the
      // next request being refused.
    } finally {
      window.location.href = "/login";
    }
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              className="data-[state=open]:bg-sidebar-accent"
              tooltip={user.name}
            >
              <Avatar name={user.name} />
              <div className="grid flex-1 text-left leading-tight">
                <span className="truncate text-meta font-medium">
                  {user.name}
                </span>
                <span className="truncate text-meta text-sidebar-foreground/55">
                  {user.email}
                </span>
              </div>
              <ChevronsUpDown className="ml-auto size-3.5 text-sidebar-foreground/45" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>

          <DropdownMenuContent side="top" align="start" className="w-56">
            <DropdownMenuLabel className="flex items-center gap-2 py-2">
              <Avatar name={user.name} />
              <div className="grid flex-1 leading-tight">
                <span className="truncate text-meta font-medium text-foreground">
                  {user.name}
                </span>
                <span className="truncate text-meta font-normal">
                  {user.email}
                </span>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/settings">
                <Settings />
                Settings
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link href="/identities">
                <KeyRound />
                Identities
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              destructive
              onSelect={() => {
                toast.success("Signing out…");
                void signOut();
              }}
            >
              <LogOut />
              Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}

