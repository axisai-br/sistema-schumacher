import { Navbar, NavbarBrand, NavbarContent, NavbarItem, NavbarMenuToggle, NavbarMenu, NavbarMenuItem, Button, Dropdown, DropdownTrigger, DropdownMenu, DropdownItem } from '@heroui/react'
import { useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { ChevronDown, MapPin, Menu, X } from 'lucide-react'

const menuItems = [
    { name: 'Sobre', href: '#sobre', isAnchor: true },
    { name: 'Frota', href: '#frota', isAnchor: true },
    { name: 'Orçamento', href: '/orcamento', isAnchor: false },
    { name: 'Depoimentos', href: '#depoimentos', isAnchor: true },
]

const destinations = [
    { name: '🏝️ Lençóis Maranhenses', href: '/viagens/maranhao', badge: 'Mais procurado' },
    { name: '🎢 Santa Catarina', href: '/viagens/santa-catarina', badge: null },
]

function scrollToElement(selector) {
    const behavior = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
    document.querySelector(selector)?.scrollIntoView({ behavior })
}

export default function Header() {
    const [isMenuOpen, setIsMenuOpen] = useState(false)
    const location = useLocation()
    const navigate = useNavigate()
    const isHome = location.pathname === '/'

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

    const handleNavigation = (item) => {
        if (item.isAnchor) {
            if (!isHome) {
                navigate('/', { state: { scrollTarget: item.href } })
            } else {
                scrollToElement(item.href)
            }
        } else {
            navigate(item.href)
        }
        setIsMenuOpen(false)
    }

    return (
        <Navbar
            isMenuOpen={isMenuOpen}
            onMenuOpenChange={setIsMenuOpen}
            height="5rem"
            className="fixed top-0 z-50 h-20 border-b border-brand-blue-grey/40 bg-brand-lynx/95 backdrop-blur-md"
            classNames={{ wrapper: 'h-full', srOnly: 'sr-only' }}
            maxWidth="xl"
        >
            <NavbarContent className="h-full min-w-0 gap-2" justify="start">
                <NavbarMenuToggle
                    aria-label={isMenuOpen ? "Fechar menu" : "Abrir menu"}
                    className="shrink-0 text-brand-gunship focus-visible:ring-2 focus-visible:ring-brand-ember lg:hidden"
                    icon={(open) => open ? <X aria-hidden="true" size={24} /> : <Menu aria-hidden="true" size={24} />}
                />
                <NavbarBrand className="min-w-0">
                    <Link
                        to="/"
                        onClick={() => setIsMenuOpen(false)}
                        aria-label="Página inicial — Schumacher Tur"
                        className="rounded-sm font-heading text-lg font-semibold tracking-[-0.02em] text-brand-gunship focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ember focus-visible:ring-offset-4 focus-visible:ring-offset-brand-lynx sm:text-xl"
                    >
                        Schumacher Tur
                    </Link>
                </NavbarBrand>
            </NavbarContent>

            <NavbarContent className="hidden h-full gap-6 lg:flex" justify="center">
                {/* Dropdown Viagens */}
                <Dropdown
                    classNames={{
                        content: "border border-brand-blue-grey/50 bg-brand-lynx shadow-lg",
                    }}
                >
                    <NavbarItem>
                        <DropdownTrigger>
                            <Button
                                disableRipple
                                className="rounded-sm bg-transparent p-2 font-medium text-brand-gunship data-[hover=true]:bg-brand-ember/10 focus-visible:ring-2 focus-visible:ring-brand-ember"
                                endContent={<ChevronDown size={16} />}
                                variant="light"
                            >
                                Viagens
                            </Button>
                        </DropdownTrigger>
                    </NavbarItem>
                    <DropdownMenu
                        aria-label="Destinos"
                        className="w-64"
                        itemClasses={{
                            base: [
                                "gap-4",
                                "transition-colors",
                                "data-[hover=true]:bg-brand-ember/10",
                                "data-[hover=true]:text-brand-gunship",
                            ],
                            title: "font-semibold",
                            description: "text-brand-gunship/75 text-xs",
                        }}
                    >
                        {destinations.map((dest) => (
                            <DropdownItem
                                key={dest.href}
                                description={dest.badge}
                                startContent={<MapPin size={18} className="text-brand-ember" />}
                                onClick={() => navigate(dest.href)}
                            >
                                {dest.name}
                            </DropdownItem>
                        ))}
                    </DropdownMenu>
                </Dropdown>

                {/* Menu Items normais */}
                {menuItems.map((item) => (
                    <NavbarItem key={item.name}>
                        <button
                            onClick={() => handleNavigation(item)}
                            className="rounded-sm px-2 py-2 font-medium text-brand-gunship transition-colors duration-200 hover:bg-brand-ember/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ember focus-visible:ring-offset-4 focus-visible:ring-offset-brand-lynx"
                        >
                            {item.name}
                        </button>
                    </NavbarItem>
                ))}
            </NavbarContent>

            <NavbarContent className="hidden h-full lg:flex" justify="end">
                <NavbarItem>
                    <Button
                        size="sm"
                        className="bg-brand-ember px-6 font-semibold text-black transition-[filter] hover:brightness-95 focus-visible:ring-2 focus-visible:ring-brand-ember focus-visible:ring-offset-2 focus-visible:ring-offset-brand-lynx"
                        onClick={() => navigate('/orcamento')}
                    >
                        Solicitar Orçamento
                    </Button>
                </NavbarItem>
            </NavbarContent>

            {/* Mobile Menu */}
            <NavbarMenu className="bottom-auto top-20 bg-brand-lynx/95 pt-6 backdrop-blur-md lg:hidden">
                {/* Viagens no mobile */}
                <NavbarMenuItem>
                    <p className="mb-2 mt-2 text-xs uppercase tracking-[0.18em] text-brand-gunship/70">Viagens</p>
                </NavbarMenuItem>
                {destinations.map((dest) => (
                    <NavbarMenuItem key={dest.href}>
                        <Link
                            to={dest.href}
                            onClick={() => setIsMenuOpen(false)}
                            className="flex w-full items-center gap-2 rounded-sm py-2 text-left text-lg font-medium text-brand-gunship transition-colors hover:bg-brand-ember/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ember"
                        >
                            {dest.name}
                            {dest.badge && (
                                <span className="rounded-full bg-brand-ember/15 px-2 py-0.5 text-xs text-brand-gunship">
                                    {dest.badge}
                                </span>
                            )}
                        </Link>
                    </NavbarMenuItem>
                ))}

                <NavbarMenuItem>
                    <div className="my-4 border-t border-brand-blue-grey/40" />
                </NavbarMenuItem>

                {menuItems.map((item, index) => (
                    <NavbarMenuItem key={`${item.name}-${index}`}>
                        <button
                            onClick={() => handleNavigation(item)}
                            className="w-full rounded-sm px-2 py-3 text-left text-lg font-medium text-brand-gunship transition-colors hover:bg-brand-ember/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ember"
                        >
                            {item.name}
                        </button>
                    </NavbarMenuItem>
                ))}

                <NavbarMenuItem className="pt-4">
                    <Button
                        className="w-full bg-brand-ember font-semibold text-black transition-[filter] hover:brightness-95 focus-visible:ring-2 focus-visible:ring-brand-ember focus-visible:ring-offset-2 focus-visible:ring-offset-brand-lynx"
                        onClick={() => {
                            navigate('/orcamento')
                            setIsMenuOpen(false)
                        }}
                    >
                        Solicitar Orçamento
                    </Button>
                </NavbarMenuItem>
            </NavbarMenu>
        </Navbar>
    )
}
