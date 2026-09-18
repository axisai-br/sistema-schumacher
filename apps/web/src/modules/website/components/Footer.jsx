import {
    Facebook,
    Heart,
    Instagram,
    Mail,
    MapPin,
    MessageCircle,
    Phone,
    Youtube,
} from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import schumacherHorizontalLight from '../../../assets/brand/logos/schumacher-horizontal-light.svg'
import { contacts, createTelUrl, createWhatsAppUrl } from '../data/contacts'

const links = {
    empresa: [
        { name: 'Sobre', href: '#sobre' },
        { name: 'Serviços', href: '#servicos' },
        { name: 'Nossa Frota', href: '#frota' },
        { name: 'Depoimentos', href: '#depoimentos' },
        { name: 'Solicitar Orçamento', href: '/orcamento', isRoute: true },
    ],
    contato: [
        { name: contacts.commercial.display, href: createTelUrl(contacts.commercial), icon: <Phone aria-hidden="true" className="shrink-0" size={18} strokeWidth={1.8} /> },
        { name: contacts.secondaryWhatsApp.display, href: createWhatsAppUrl(contacts.secondaryWhatsApp), icon: <MessageCircle aria-hidden="true" className="shrink-0" size={18} strokeWidth={1.8} /> },
        { name: 'turismo@schumacher.tur.br', href: 'mailto:turismo@schumacher.tur.br', icon: <Mail aria-hidden="true" className="shrink-0" size={18} strokeWidth={1.8} /> },
        { name: 'SC-355, KM 35 - Fraiburgo/SC', href: 'https://maps.google.com/?q=SC-355+KM+35+Fraiburgo+SC', icon: <MapPin aria-hidden="true" className="shrink-0" size={18} strokeWidth={1.8} /> },
    ],
}

const socialLinks = [
    { label: 'Facebook', href: 'https://www.facebook.com/joseane.schumachertur', icon: <Facebook aria-hidden="true" size={20} strokeWidth={1.8} /> },
    { label: 'Instagram', href: 'https://www.instagram.com/schumacher_tur/', icon: <Instagram aria-hidden="true" size={20} strokeWidth={1.8} /> },
    { label: 'YouTube', href: 'https://www.youtube.com/channel/UCZV5YZpjW_7QtGuHre6Tj8w', icon: <Youtube aria-hidden="true" size={20} strokeWidth={1.8} /> },
]

function getScrollBehavior() {
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
}

export default function Footer() {
    const navigate = useNavigate()

    const handleNavigation = (link) => {
        const behavior = getScrollBehavior()

        if (link.isRoute) {
            navigate(link.href)
            window.scrollTo({ top: 0, behavior })
        } else if (link.href.startsWith('#')) {
            if (window.location.pathname !== '/') {
                navigate('/')
                setTimeout(() => {
                    document.querySelector(link.href)?.scrollIntoView({ behavior })
                }, 100)
            } else {
                document.querySelector(link.href)?.scrollIntoView({ behavior })
            }
        }
    }

    return (
        <footer id="contato" className="border-t-4 border-brand-ember bg-brand-gunship text-brand-lynx">
            <div className="mx-auto max-w-7xl px-5 py-16 sm:px-8 sm:py-20 lg:px-10">
                <div className="grid gap-12 md:grid-cols-3 md:gap-10 lg:gap-16">
                    <div className="min-w-0">
                        <div className="relative mb-5 h-12 w-[230px] overflow-hidden">
                            <img
                                src={schumacherHorizontalLight}
                                alt="Schumacher Tur"
                                className="absolute left-1/2 top-1/2 w-[290px] max-w-none -translate-x-1/2 -translate-y-1/2"
                            />
                        </div>
                        <p className="mb-5 max-w-sm text-sm leading-7 text-brand-lynx/75 sm:text-base">
                            Sua viagem com conforto, segurança e pontualidade.
                            Especialistas em viagens ao Maranhão e turismo em Santa Catarina.
                        </p>
                        <p className="mb-7 text-xs font-medium tracking-wide text-brand-lynx/70">
                            CNPJ: 17.246.217/0001-89
                        </p>
                        <div className="flex gap-3">
                            {socialLinks.map(({ label, href, icon }) => (
                                <a
                                    key={label}
                                    href={href}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    aria-label={label}
                                    className="flex h-11 w-11 items-center justify-center border border-brand-blue-grey/60 text-brand-lynx transition-colors hover:border-brand-lynx hover:bg-brand-lynx/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-lynx focus-visible:ring-offset-2 focus-visible:ring-offset-brand-gunship"
                                >
                                    {icon}
                                </a>
                            ))}
                        </div>
                    </div>

                    <div>
                        <h2 className="mb-5 text-sm font-bold uppercase tracking-[0.16em] text-brand-lynx">
                            Navegação
                        </h2>
                        <ul className="space-y-1">
                            {links.empresa.map((link) => (
                                <li key={link.name}>
                                    <button
                                        type="button"
                                        onClick={() => handleNavigation(link)}
                                        className="min-h-11 rounded-sm px-1 text-left text-sm font-medium text-brand-lynx/75 transition-colors hover:text-brand-lynx focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-lynx focus-visible:ring-offset-2 focus-visible:ring-offset-brand-gunship sm:text-base"
                                    >
                                        {link.name}
                                    </button>
                                </li>
                            ))}
                        </ul>
                    </div>

                    <div className="min-w-0">
                        <h2 className="mb-5 text-sm font-bold uppercase tracking-[0.16em] text-brand-lynx">
                            Contato
                        </h2>
                        <ul className="space-y-2">
                            {links.contato.map(({ name, href, icon }) => (
                                <li key={name}>
                                    <a
                                        href={href}
                                        className="flex min-h-11 min-w-0 items-center gap-3 rounded-sm px-1 text-sm text-brand-lynx/75 transition-colors hover:text-brand-lynx focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-lynx focus-visible:ring-offset-2 focus-visible:ring-offset-brand-gunship sm:text-base"
                                    >
                                        {icon}
                                        <span className="min-w-0 break-words">{name}</span>
                                    </a>
                                </li>
                            ))}
                        </ul>
                    </div>
                </div>

                <div className="my-9 h-px bg-brand-blue-grey/40" aria-hidden="true" />

                <div className="flex flex-col gap-4 text-sm text-brand-lynx/70 sm:flex-row sm:items-center sm:justify-between">
                    <p>© {new Date().getFullYear()} Schumacher Tur. Todos os direitos reservados.</p>
                    <p className="flex items-center gap-1.5">
                        Feito com <Heart aria-hidden="true" className="fill-brand-wasp text-brand-wasp" size={15} /> em Santa Catarina
                    </p>
                </div>
            </div>
        </footer>
    )
}
