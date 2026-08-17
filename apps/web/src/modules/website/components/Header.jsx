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
            className="fixed top-0 z-50 h-16 border-b border-gold-100 bg-white/90 backdrop-blur-lg"
            classNames={{ wrapper: 'h-full', srOnly: 'sr-only' }}
            maxWidth="xl"
        >
            <NavbarContent className="h-full min-w-0 gap-2" justify="start">
                <NavbarMenuToggle
                    aria-label={isMenuOpen ? "Fechar menu" : "Abrir menu"}
                    className="lg:hidden shrink-0 text-gold-500"
                    icon={(open) => open ? <X aria-hidden="true" size={24} /> : <Menu aria-hidden="true" size={24} />}
                />
                <NavbarBrand className="min-w-0">
                    <Link
                        to="/"
                        onClick={() => setIsMenuOpen(false)}
                        className="whitespace-nowrap font-heading font-bold text-xl text-dark-900 sm:text-2xl"
                    >
                        Schumacher <span className="text-gold-500">Tur</span>
                    </Link>
                </NavbarBrand>
            </NavbarContent>

            <NavbarContent className="hidden h-full gap-6 lg:flex" justify="center">
                {/* Dropdown Viagens */}
                <Dropdown
                    classNames={{
                        content: "bg-white/70 backdrop-blur-xl shadow-xl border border-white/20",
                    }}
                >
                    <NavbarItem>
                        <DropdownTrigger>
                            <Button
                                disableRipple
                                className="p-0 bg-transparent data-[hover=true]:bg-transparent text-dark-600 hover:text-gold-500 font-medium"
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
                                "data-[hover=true]:bg-gold-50/50",
                                "data-[hover=true]:text-gold-700",
                            ],
                            title: "font-semibold",
                            description: "text-gold-600/70 text-xs",
                        }}
                    >
                        {destinations.map((dest) => (
                            <DropdownItem
                                key={dest.href}
                                description={dest.badge}
                                startContent={<MapPin size={18} className="text-gold-500" />}
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
                            className="rounded text-dark-600 hover:text-gold-500 font-medium transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400 focus-visible:ring-offset-4"
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
                        className="bg-gold-400 text-white font-semibold px-6 hover:bg-gold-500 transition-colors"
                        onClick={() => navigate('/orcamento')}
                    >
                        Solicitar Orçamento
                    </Button>
                </NavbarItem>
            </NavbarContent>

            {/* Mobile Menu */}
            <NavbarMenu className="bg-white/95 backdrop-blur-lg pt-6 lg:hidden">
                {/* Viagens no mobile */}
                <NavbarMenuItem>
                    <p className="text-xs uppercase tracking-wider text-dark-400 mb-2 mt-2">Viagens</p>
                </NavbarMenuItem>
                {destinations.map((dest) => (
                    <NavbarMenuItem key={dest.href}>
                        <Link
                            to={dest.href}
                            onClick={() => setIsMenuOpen(false)}
                            className="w-full text-left py-2 text-lg text-dark-700 hover:text-gold-500 font-medium transition-colors flex items-center gap-2"
                        >
                            {dest.name}
                            {dest.badge && (
                                <span className="text-xs bg-gold-100 text-gold-600 px-2 py-0.5 rounded-full">
                                    {dest.badge}
                                </span>
                            )}
                        </Link>
                    </NavbarMenuItem>
                ))}

                <NavbarMenuItem>
                    <div className="border-t border-light-200 my-4" />
                </NavbarMenuItem>

                {menuItems.map((item, index) => (
                    <NavbarMenuItem key={`${item.name}-${index}`}>
                        <button
                            onClick={() => handleNavigation(item)}
                            className="w-full rounded-lg px-2 py-3 text-left text-lg text-dark-700 hover:text-gold-500 font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400"
                        >
                            {item.name}
                        </button>
                    </NavbarMenuItem>
                ))}

                <NavbarMenuItem className="pt-4">
                    <Button
                        className="w-full bg-gold-400 text-white font-semibold hover:bg-gold-500 transition-colors"
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
