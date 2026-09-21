import { Link } from 'react-router-dom'
import { contacts, createWhatsAppUrl } from '../data/contacts'
import { serviceCatalog } from '../data/services'

const linkClassName = 'inline-flex min-h-12 items-center rounded-sm py-3 text-sm font-semibold underline decoration-brand-ember decoration-2 underline-offset-4 hover:decoration-brand-lynx focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-lynx focus-visible:ring-offset-4 focus-visible:ring-offset-brand-gunship'

export default function Services() {
    return (
        <section id="servicos" aria-labelledby="services-title" className="bg-brand-gunship px-5 py-20 text-brand-lynx sm:px-8 sm:py-24 lg:px-10 lg:py-28">
            <div className="mx-auto max-w-7xl">
                <header className="mb-12 max-w-3xl sm:mb-16">
                    <h2 id="services-title" className="text-3xl font-bold leading-tight tracking-[-0.025em] text-brand-lynx sm:text-4xl lg:text-5xl">
                        Nossos <span className="border-b-4 border-brand-ember">Serviços</span>
                    </h2>
                </header>

                <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
                    {serviceCatalog.map(({ title, featuredId, summary, destinations }) => (
                        <article key={title} className="flex min-w-0 flex-col border border-brand-blue-grey/55 p-6 sm:p-8">
                            <h3 className="text-xl font-bold text-brand-lynx sm:text-2xl">{title}</h3>
                            {summary && <p className="mt-4 text-base leading-7 text-brand-lynx/85">{summary}</p>}
                            <div className="mt-auto pt-5">
                                {featuredId ? (
                                    <a href={`#${featuredId}`} aria-label={`Ver detalhes de ${title}`} className={linkClassName}>
                                        Ver detalhes
                                    </a>
                                ) : (
                                    <a
                                        href={createWhatsAppUrl(contacts.commercial, `Olá! Gostaria de falar sobre ${title}.`)}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        aria-label={`Falar com o comercial sobre ${title} pelo WhatsApp`}
                                        className={linkClassName}
                                    >
                                        Falar com o comercial
                                    </a>
                                )}
                                {destinations && (
                                    <div className="mt-5 border-t border-brand-blue-grey/55 pt-5">
                                        <p className="text-sm text-brand-lynx/85">Páginas de viagem</p>
                                        <ul className="mt-2 space-y-1">
                                            {destinations.map(destination => (
                                                <li key={destination.to}>
                                                    <Link to={destination.to} className={linkClassName}>
                                                        {destination.title}
                                                    </Link>
                                                </li>
                                            ))}
                                        </ul>
                                    </div>
                                )}
                            </div>
                        </article>
                    ))}
                </div>
            </div>
        </section>
    )
}
