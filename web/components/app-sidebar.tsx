"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { motion } from "motion/react";
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
  X,
} from "lucide-react";

import {
  AnimatedSidebar,
  AnimatedSidebarClose,
  AnimatedSidebarContent,
  AnimatedSidebarFooter,
  AnimatedSidebarGroup,
  AnimatedSidebarGroupContent,
  AnimatedSidebarGroupLabel,
  AnimatedSidebarHeader,
  AnimatedSidebarMenu,
  AnimatedSidebarMenuButton,
  AnimatedSidebarMenuItem,
  AnimatedSidebarRail,
} from "@/components/motion/animated-sidebar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SignOutDialog } from "@/components/sign-out-dialog";
import { Avatar } from "@/components/ui/avatar";
import type { SessionUser } from "@/lib/session-user";

/**
 * Client-side navigation. The beUI menu button renders a plain anchor for an
 * `href`, which would reload the whole application on every move between
 * sections; `linkComponent` swaps in the router-aware link instead.
 */
const MotionLink = motion.create(Link);

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

const SUPPORT: NavItem[] = [
  { href: "/docs", label: "Docs", icon: LifeBuoy },
  { href: "/settings", label: "Settings", icon: Settings },
];

export function AppSidebar({ user }: { user: SessionUser }) {
  const pathname = usePathname();

  const isActive = (href: string) =>
    pathname === href || pathname.startsWith(`${href}/`);

  return (
    <AnimatedSidebar
      ariaLabel="Warder navigation"
      collapsible="icon"
      panelClassName="border-border"
    >
      <AnimatedSidebarHeader className="p-3 pb-2">
        <div className="flex min-h-11 items-center gap-3 overflow-hidden px-2">
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <span className="truncate text-body font-semibold text-foreground">
              {user.organization}
            </span>
            <ChevronsUpDown
              aria-hidden="true"
              className="size-3.5 shrink-0 text-muted-foreground"
            />
          </div>
          <AnimatedSidebarClose className="ml-auto text-muted-foreground hover:bg-muted md:hidden">
            <X aria-hidden="true" className="size-4" />
          </AnimatedSidebarClose>
        </div>
      </AnimatedSidebarHeader>

      <AnimatedSidebarContent className="px-2 pt-1">
        <NavGroup label="Platform" items={PLATFORM} isActive={isActive} />
        <NavGroup label="Governance" items={GOVERNANCE} isActive={isActive} />
        <NavGroup label="Support" items={SUPPORT} isActive={isActive} />
      </AnimatedSidebarContent>

      <AnimatedSidebarFooter>
        <UserMenu user={user} />
      </AnimatedSidebarFooter>

      <AnimatedSidebarRail />
    </AnimatedSidebar>
  );
}

function NavGroup({
  label,
  items,
  isActive,
}: {
  label: string;
  items: NavItem[];
  isActive: (href: string) => boolean;
}) {
  return (
    <AnimatedSidebarGroup>
      <AnimatedSidebarGroupLabel>{label}</AnimatedSidebarGroupLabel>
      <AnimatedSidebarGroupContent>
        <AnimatedSidebarMenu>
          {items.map(({ href, label: itemLabel, icon: Icon }) => (
            <AnimatedSidebarMenuItem key={href}>
              <AnimatedSidebarMenuButton
                href={href}
                linkComponent={MotionLink}
                isActive={isActive(href)}
                icon={<Icon className="size-4" />}
              >
                {itemLabel}
              </AnimatedSidebarMenuButton>
            </AnimatedSidebarMenuItem>
          ))}
        </AnimatedSidebarMenu>
      </AnimatedSidebarGroupContent>
    </AnimatedSidebarGroup>
  );
}

function UserMenu({ user }: { user: SessionUser }) {
  const [confirming, setConfirming] = useState(false);

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            title={user.name}
            className="flex w-full min-w-0 items-center gap-2.5 rounded-xl px-2 py-1.5 text-left outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:bg-muted"
          >
            <Avatar name={user.name} />
            <div className="grid min-w-0 flex-1 leading-tight group-data-[state=collapsed]/sidebar:hidden">
              <span className="truncate text-meta font-medium text-foreground">
                {user.name}
              </span>
              <span className="truncate text-meta text-muted-foreground">
                {user.email}
              </span>
            </div>
            <ChevronsUpDown className="ml-auto size-3.5 shrink-0 text-muted-foreground group-data-[state=collapsed]/sidebar:hidden" />
          </button>
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
          <DropdownMenuItem destructive onSelect={() => setConfirming(true)}>
            <LogOut />
            Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {/*
      A sibling of the menu, not a child: Radix unmounts the menu content when
      the menu closes, and the modal would go with it.
    */}
      <SignOutDialog open={confirming} onOpenChange={setConfirming} />
    </>
  );
}
