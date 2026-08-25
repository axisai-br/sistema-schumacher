import { Quote, Star, UserRound, UsersRound } from 'lucide-react'

const testimonials = [
    {
        name: 'Darlene Xavier',
        role: 'Viagem ao Maranhão',
        company: 'Lençóis Maranhenses',
        quote: 'Gostaria de agradecer por tudo, principalmente pela paciência. A viagem foi perfeita… Receptivo maravilhoso, motorista maravilhoso, guia maravilhoso, funcionários e serviços perfeitos. Sem contar o lugar visitado que é simplesmente magnífico e mágico.',
        icon: <UserRound aria-hidden="true" size={22} strokeWidth={1.8} />,
    },
    {
        name: 'Carlos Alberto',
        role: 'Viagem ao Maranhão',
        company: 'Experiência Inesquecível',
        quote: 'A experiência no Maranhão foi fantástica e completamente impactante. Os lugares são incríveis e deslumbrantes. Vale lembrar que é um roteiro simples mas magnífico, cheio de natureza e interação com o ambiente! As pessoas são acolhedoras e fazem você se sentir em casa, literalmente.',
        icon: <UserRound aria-hidden="true" size={22} strokeWidth={1.8} />,
    },
    {
        name: 'Família Campos',
        role: 'Turismo em Grupo',
        company: 'Viagem em Família',
        quote: 'A viagem foi maravilhosa. Além das belezas naturais, tiro o chapéu para a organização. Tudo feito com muito profissionalismo, pensando nos mínimos detalhes. Super recomendo.',
        icon: <UsersRound aria-hidden="true" size={22} strokeWidth={1.8} />,
    },
]

export default function Testimonials() {
    return (
        <section id="depoimentos" className="bg-brand-gunship px-5 py-20 sm:px-8 sm:py-24 lg:px-10 lg:py-28">
            <div className="mx-auto max-w-7xl">
                <header className="mb-12 max-w-3xl sm:mb-16">
                    <h2 className="text-3xl font-bold leading-tight tracking-[-0.025em] text-brand-lynx sm:text-4xl lg:text-5xl">
                        Quem Viaja, <span className="border-b-4 border-brand-ember">Recomenda</span>
                    </h2>
                    <p className="mt-5 text-base leading-7 text-brand-lynx/75 sm:text-lg">
                        Veja o que nossos clientes dizem sobre nós
                    </p>
                </header>

                <div className="grid gap-px border border-brand-blue-grey/45 bg-brand-blue-grey/45 md:grid-cols-3">
                    {testimonials.map(({ name, role, company, quote, icon }) => (
                        <article key={name} className="flex h-full flex-col bg-brand-gunship p-6 sm:p-8">
                            <div className="mb-7 flex items-start justify-between gap-4">
                                <div className="flex gap-1" role="img" aria-label="5 de 5 estrelas">
                                    {Array.from({ length: 5 }, (_, index) => (
                                        <Star
                                            key={index}
                                            aria-hidden="true"
                                            className="fill-brand-wasp text-brand-wasp"
                                            size={17}
                                            strokeWidth={1.5}
                                        />
                                    ))}
                                </div>
                                <Quote aria-hidden="true" className="text-brand-lynx/45" size={30} strokeWidth={1.5} />
                            </div>

                            <blockquote className="mb-8 flex-1 text-base leading-7 text-brand-lynx/85">
                                “{quote}”
                            </blockquote>

                            <div className="flex items-center gap-4 border-t border-brand-blue-grey/35 pt-6">
                                <span className="flex h-11 w-11 shrink-0 items-center justify-center bg-brand-lynx text-brand-gunship">
                                    {icon}
                                </span>
                                <div>
                                    <p className="font-bold text-brand-lynx">{name}</p>
                                    <p className="mt-0.5 text-sm text-brand-lynx/65">{role}</p>
                                    <p className="text-sm font-semibold text-brand-lynx">{company}</p>
                                </div>
                            </div>
                        </article>
                    ))}
                </div>
            </div>
        </section>
    )
}
