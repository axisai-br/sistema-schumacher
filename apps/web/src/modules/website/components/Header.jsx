import {
    Button,
    Dropdown,
    DropdownItem,
    DropdownMenu,
    DropdownTrigger,
    Navbar,
    NavbarBrand,
    NavbarContent,
    NavbarItem,
    NavbarMenu,
    NavbarMenuItem,
    NavbarMenuToggle,
} from '@heroui/react'
import { useEffect, useRef, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { ChevronDown, MapPin, Menu, X } from 'lucide-react'
import schumacherHorizontalDark from '../../../assets/brand/logos/schumacher-horizontal-dark.svg'
import { contacts, createWhatsAppUrl } from '../data/contacts'

const HEADER_HEIGHT = 80

const destinations = [
    { name: 'Lençóis Maranhenses', href: '/viagens/maranhao', badge: 'Mais procurado' },
    { name: 'Santa Catarina', href: '/viagens/santa-catarina', badge: null },
]

const events = [
    { name: 'Transfers', href: '#eventos-transfers', isAnchor: true },
]

const primaryLinks = [
    { name: 'Orçamento', href: '/orcamento', isAnchor: false },
    { name: 'Depoimentos', href: '#depoimentos', isAnchor: true },
]

function WhatsAppIcon({ className = '' }) {
    return (
        <svg
            aria-hidden="true"
            className={className}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
        >
            <path d="M20.2 11.8a8.2 8.2 0 0 1-12.1 7.2L3.8 20.2 5 16a8.2 8.2 0 1 1 15.2-4.2Z" />
            <path d="M8.2 7.7c.2-.4.4-.4.7-.4h.5c.2 0 .4.1.5.4l.8 1.8c.1.3 0 .5-.2.7l-.6.7c.8 1.5 1.9 2.5 3.4 3.2l.7-.9c.2-.2.4-.3.7-.2l1.9.9c.3.1.4.3.4.6v.4c0 .4-.2.8-.5 1-1 .7-2.4.7-3.9 0-2.5-1.1-4.5-3.1-5.5-5.6-.5-1.1-.3-2 .1-2.6Z" />
        </svg>
    )
}

function scrollToElement(selector) {
    const target = document.querySelector(selector)
    if (!target) return

    const behavior = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
    const top = window.scrollY + target.getBoundingClientRect().top - HEADER_HEIGHT
    window.scrollTo({ top: Math.max(0, top), behavior })
}

export default function Header() {
    const [isMenuOpen, setIsMenuOpen] = useState(false)
    const [mobileGroup, setMobileGroup] = useState(null)
    const [desktopDropdown, setDesktopDropdown] = useState(null)
    const menuToggleRef = useRef(null)
    const mobileGroupRefs = useRef({})
    const primaryLinkRefs = useRef({})
    const pendingDesktopFocusRef = useRef(null)
    const location = useLocation()
    const navigate = useNavigate()
    const isHome = location.pathname === '/'
    const whatsappUrl = createWhatsAppUrl(contacts.commercial)

    useEffect(() => {
        const scrollTarget = location.state?.scrollTarget
        if (!isHome || !scrollTarget) return undefined

        let secondFrame
        const firstFrame = window.requestAnimationFrame(() => {
            secondFrame = window.requestAnimationFrame(() => {
                scrollToElement(scrollTarget)
                navigate(location.pathname, { replace: true, state: null })
            })
        })

        return () => {
            window.cancelAnimationFrame(firstFrame)
            window.cancelAnimationFrame(secondFrame)
        }
    }, [isHome, location.pathname, location.state, navigate])

    useEffect(() => {
        if (!desktopDropdown) return undefined

        const handleDesktopKeyDown = (event) => {
            const openDropdown = desktopDropdown

            if (event.key === 'Tab' && document.activeElement?.closest('[role="menu"]')) {
                const nextControl = event.shiftKey
                    ? document.getElementById(`desktop-${openDropdown}-trigger`)
                    : openDropdown === 'travel'
                        ? document.getElementById('desktop-events-trigger')
                        : primaryLinkRefs.current['Orçamento']

                event.preventDefault()
                event.stopPropagation()
                pendingDesktopFocusRef.current = { target: nextControl }
                setDesktopDropdown(null)
                return
            }

            if (event.key !== 'Escape') return

            const trigger = document.getElementById(`desktop-${openDropdown}-trigger`)
            event.preventDefault()
            event.stopPropagation()
            pendingDesktopFocusRef.current = { target: trigger }
            setDesktopDropdown(null)
        }

        window.addEventListener('keydown', handleDesktopKeyDown, true)
        return () => window.removeEventListener('keydown', handleDesktopKeyDown, true)
    }, [desktopDropdown])

    useEffect(() => {
        if (desktopDropdown || !pendingDesktopFocusRef.current) return undefined

        const focusRequest = pendingDesktopFocusRef.current
        let animationFrame
        const focusAfterMenuUnmount = () => {
            if (pendingDesktopFocusRef.current !== focusRequest) return

            const desktopMenu = document.querySelector(
                '[role="menu"][aria-label="Viagens"], [role="menu"][aria-label="Eventos"]',
            )

            if (desktopMenu) {
                animationFrame = window.requestAnimationFrame(focusAfterMenuUnmount)
                return
            }

            pendingDesktopFocusRef.current = null
            focusRequest.target?.focus()
        }

        animationFrame = window.requestAnimationFrame(focusAfterMenuUnmount)
        return () => window.cancelAnimationFrame(animationFrame)
    }, [desktopDropdown])

    const closeMobileMenu = () => {
        setIsMenuOpen(false)
        setMobileGroup(null)
    }

    const handleMenuOpenChange = (open) => {
        setIsMenuOpen(open)
        if (!open) setMobileGroup(null)
    }

    const handleDesktopOpenChange = (group, open) => {
        if (open) pendingDesktopFocusRef.current = null
        setDesktopDropdown(open ? group : null)
    }

    const handleNavigation = (item) => {
        const desktopTrigger = desktopDropdown
            ? document.getElementById(`desktop-${desktopDropdown}-trigger`)
            : null
        pendingDesktopFocusRef.current = desktopTrigger ? { target: desktopTrigger } : null
        setDesktopDropdown(null)
        closeMobileMenu()

        if (!item.isAnchor) {
            navigate(item.href)
            return
        }

        if (isHome) {
            window.requestAnimationFrame(() => {
                window.requestAnimationFrame(() => scrollToElement(item.href))
            })
            return
        }

        navigate('/', { state: { scrollTarget: item.href } })
    }

    const handleMobileGroupToggle = (group) => {
        setMobileGroup(current => current === group ? null : group)
    }

    const handleMobileMenuKeyDown = (event) => {
        if (event.key !== 'Escape') return

        event.preventDefault()
        event.stopPropagation()

        if (mobileGroup) {
            const openGroup = mobileGroup
            setMobileGroup(null)
            window.requestAnimationFrame(() => mobileGroupRefs.current[openGroup]?.focus())
            return
        }

        closeMobileMenu()
        window.requestAnimationFrame(() => menuToggleRef.current?.focus())
    }

    const renderMobileGroup = (group, label, items) => {
        const isOpen = mobileGroup === group
        const panelId = `mobile-${group}-menu`

        return (
            <NavbarMenuItem>
                <button
                    ref={element => { mobileGroupRefs.current[group] = element }}
                    type="button"
                    aria-expanded={isOpen}
                    aria-controls={panelId}
                    onClick={() => handleMobileGroupToggle(group)}
                    className="flex min-h-11 w-full items-center justify-between rounded-sm px-2 py-2 text-left text-lg font-medium text-brand-gunship transition-colors hover:bg-brand-ember/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ember motion-reduce:transition-none"
                >
                    {label}
                    <ChevronDown
                        aria-hidden="true"
                        size={18}
                        className={`transition-transform motion-reduce:transition-none ${isOpen ? 'rotate-180' : ''}`}
                    />
                </button>
                {isOpen && (
                    <ul id={panelId} className="mt-1 space-y-1 border-l-2 border-brand-ember/70 pl-3">
                        {items.map(item => {
                            const content = (
                                <>
                                    {item.name}
                                    {item.badge && (
                                        <span className="rounded-full bg-brand-ember/15 px-2 py-0.5 text-xs text-brand-gunship">
                                            {item.badge}
                                        </span>
                                    )}
                                </>
                            )
                            const className = 'flex min-h-11 w-full items-center gap-2 rounded-sm px-2 py-2 text-left font-medium text-brand-gunship transition-colors hover:bg-brand-ember/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ember motion-reduce:transition-none'

                            return (
                                <li key={item.href}>
                                    {item.isAnchor ? (
                                        <button
                                            type="button"
                                            onClick={() => handleNavigation(item)}
                                            className={className}
                                        >
                                            {content}
                                        </button>
                                    ) : (
                                        <Link
                                            to={item.href}
                                            onClick={closeMobileMenu}
                                            className={className}
                                        >
                                            {content}
                                        </Link>
                                    )}
                                </li>
                            )
                        })}
                    </ul>
                )}
            </NavbarMenuItem>
        )
    }

    return (
        <Navbar
            isMenuOpen={isMenuOpen}
            onMenuOpenChange={handleMenuOpenChange}
            height="5rem"
            className="fixed top-0 z-50 h-20 border-b border-brand-blue-grey/40 bg-brand-lynx/95 backdrop-blur-md"
            classNames={{ wrapper: 'h-full', srOnly: 'sr-only' }}
            maxWidth="xl"
        >
            <NavbarContent className="h-full min-w-0 gap-1 sm:gap-2" justify="start">
                <NavbarMenuToggle
                    ref={menuToggleRef}
                    aria-label={isMenuOpen ? 'Fechar menu' : 'Abrir menu'}
                    className="h-11 w-11 shrink-0 text-brand-gunship focus-visible:ring-2 focus-visible:ring-brand-ember xl:hidden"
                    icon={(open) => open ? <X aria-hidden="true" size={24} /> : <Menu aria-hidden="true" size={24} />}
                />
                <NavbarBrand className="min-w-0">
                    <Link
                        to="/"
                        onClick={closeMobileMenu}
                        aria-label="Página inicial — Schumacher Tur"
                        className="flex min-h-11 items-center rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ember focus-visible:ring-offset-4 focus-visible:ring-offset-brand-lynx"
                    >
                        <span className="relative block h-10 w-14 overflow-hidden sm:hidden" aria-hidden="true">
                            <img
                                src="/assets/brand/schumacher-symbol-dark.svg"
                                alt=""
                                className="absolute left-1/2 top-1/2 w-[72px] max-w-none -translate-x-1/2 -translate-y-1/2"
                            />
                        </span>
                        <span className="relative hidden h-10 w-[190px] overflow-hidden sm:block" aria-hidden="true">
                            <img
                                src={schumacherHorizontalDark}
                                alt=""
                                className="absolute left-1/2 top-1/2 w-[237px] max-w-none -translate-x-1/2 -translate-y-1/2"
                            />
                        </span>
                    </Link>
                </NavbarBrand>
            </NavbarContent>

            <NavbarContent className="hidden h-full gap-3 xl:flex 2xl:gap-5" justify="center">
                <Dropdown
                    isOpen={desktopDropdown === 'travel'}
                    onOpenChange={open => handleDesktopOpenChange('travel', open)}
                    placement="bottom-start"
                    classNames={{ content: 'border border-brand-blue-grey/50 bg-brand-lynx shadow-lg' }}
                >
                    <NavbarItem>
                        <DropdownTrigger>
                            <Button
                                id="desktop-travel-trigger"
                                disableRipple
                                className="min-h-11 rounded-sm bg-transparent px-3 font-medium text-brand-gunship data-[hover=true]:bg-brand-ember/10 focus-visible:ring-2 focus-visible:ring-brand-ember"
                                endContent={<ChevronDown aria-hidden="true" size={16} />}
                                variant="light"
                            >
                                Viagens
                            </Button>
                        </DropdownTrigger>
                    </NavbarItem>
                    <DropdownMenu
                        aria-label="Viagens"
                        className="w-64"
                        shouldFocusWrap={false}
                        onAction={key => handleNavigation(destinations.find(item => item.href === key))}
                        itemClasses={{
                            base: [
                                'min-h-11 gap-4',
                                'transition-colors',
                                'data-[hover=true]:bg-brand-ember/10',
                                'data-[hover=true]:text-brand-gunship',
                            ],
                            title: 'font-semibold',
                            description: 'text-brand-gunship/75 text-xs',
                        }}
                    >
                        {destinations.map(destination => (
                            <DropdownItem
                                key={destination.href}
                                description={destination.badge}
                                startContent={<MapPin aria-hidden="true" size={18} className="text-brand-ember" />}
                            >
                                {destination.name}
                            </DropdownItem>
                        ))}
                    </DropdownMenu>
                </Dropdown>

                <Dropdown
                    isOpen={desktopDropdown === 'events'}
                    onOpenChange={open => handleDesktopOpenChange('events', open)}
                    placement="bottom-start"
                    classNames={{ content: 'border border-brand-blue-grey/50 bg-brand-lynx shadow-lg' }}
                >
                    <NavbarItem>
                        <DropdownTrigger>
                            <Button
                                id="desktop-events-trigger"
                                disableRipple
                                className="min-h-11 rounded-sm bg-transparent px-3 font-medium text-brand-gunship data-[hover=true]:bg-brand-ember/10 focus-visible:ring-2 focus-visible:ring-brand-ember"
                                endContent={<ChevronDown aria-hidden="true" size={16} />}
                                variant="light"
                            >
                                Eventos
                            </Button>
                        </DropdownTrigger>
                    </NavbarItem>
                    <DropdownMenu
                        aria-label="Eventos"
                        className="w-48"
                        shouldFocusWrap={false}
                        onAction={key => handleNavigation(events.find(item => item.href === key))}
                        itemClasses={{
                            base: [
                                'min-h-11',
                                'transition-colors',
                                'data-[hover=true]:bg-brand-ember/10',
                                'data-[hover=true]:text-brand-gunship',
                            ],
                            title: 'font-semibold',
                        }}
                    >
                        {events.map(event => (
                            <DropdownItem key={event.href}>{event.name}</DropdownItem>
                        ))}
                    </DropdownMenu>
                </Dropdown>

                {primaryLinks.map(item => (
                    <NavbarItem key={item.name}>
                        <button
                            ref={element => { primaryLinkRefs.current[item.name] = element }}
                            type="button"
                            onClick={() => handleNavigation(item)}
                            className="min-h-11 rounded-sm px-3 py-2 font-medium text-brand-gunship transition-colors duration-200 hover:bg-brand-ember/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ember focus-visible:ring-offset-4 focus-visible:ring-offset-brand-lynx motion-reduce:transition-none"
                        >
                            {item.name}
                        </button>
                    </NavbarItem>
                ))}
            </NavbarContent>

            <NavbarContent className="h-full gap-0" justify="end">
                <NavbarItem>
                    <a
                        href={whatsappUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        aria-label={`Falar com a Schumacher Tur pelo WhatsApp comercial ${contacts.commercial.display}`}
                        onClick={closeMobileMenu}
                        className="flex h-11 w-11 items-center justify-center rounded-sm text-brand-gunship transition-colors hover:bg-brand-ember/10 hover:text-brand-ember focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ember focus-visible:ring-offset-2 focus-visible:ring-offset-brand-lynx motion-reduce:transition-none"
                    >
                        <WhatsAppIcon className="h-6 w-6" />
                    </a>
                </NavbarItem>
            </NavbarContent>

            <NavbarMenu
                onKeyDown={handleMobileMenuKeyDown}
                className="bottom-auto top-20 z-40 gap-2 bg-brand-lynx/95 pt-6 backdrop-blur-md xl:hidden"
            >
                {renderMobileGroup('travel', 'Viagens', destinations)}
                {renderMobileGroup('events', 'Eventos', events)}

                <NavbarMenuItem>
                    <div className="my-2 border-t border-brand-blue-grey/40" />
                </NavbarMenuItem>

                {primaryLinks.map(item => (
                    <NavbarMenuItem key={item.name}>
                        <button
                            type="button"
                            onClick={() => handleNavigation(item)}
                            className="min-h-11 w-full rounded-sm px-2 py-2 text-left text-lg font-medium text-brand-gunship transition-colors hover:bg-brand-ember/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ember motion-reduce:transition-none"
                        >
                            {item.name}
                        </button>
                    </NavbarMenuItem>
                ))}
            </NavbarMenu>
        </Navbar>
    )
}
