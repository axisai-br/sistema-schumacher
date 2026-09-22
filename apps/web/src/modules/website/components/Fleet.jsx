import { BusFront } from 'lucide-react'
import { buses, micros } from '../data/fleet'

function FleetGroup({ title, vehicles, imageSrc, imageAlt }) {
    return (
        <div className="grid gap-7 lg:grid-cols-12 lg:gap-10">
            <div className="lg:col-span-3 lg:sticky lg:top-28 lg:self-start">
                <div className="flex items-center gap-4">
                    <span className="h-1 w-10 shrink-0 bg-brand-ember" aria-hidden="true" />
                    <h3 className="text-2xl font-bold text-brand-gunship">{title}</h3>
                </div>
                <img
                    src={imageSrc}
                    alt={imageAlt}
                    width={1672}
                    height={941}
                    loading="lazy"
                    className="mt-6 block h-auto w-full max-w-sm object-contain"
                />
            </div>

            <ul className="grid gap-px border border-brand-blue-grey/55 bg-brand-blue-grey/55 sm:grid-cols-2 lg:col-span-9 lg:grid-cols-3">
                {vehicles.map((vehicle) => (
                    <li
                        key={vehicle.id}
                        className="flex min-h-28 items-center gap-4 bg-brand-lynx px-5 py-6 transition-colors hover:bg-white/70 sm:px-6"
                    >
                        <BusFront aria-hidden="true" className="shrink-0 text-brand-ember" size={25} strokeWidth={1.8} />
                        <span className="font-heading text-base font-bold leading-snug text-brand-gunship sm:text-lg">
                            {vehicle.name}
                        </span>
                    </li>
                ))}
            </ul>
        </div>
    )
}

export default function Fleet() {
    return (
        <section id="frota" className="border-b border-brand-blue-grey/35 bg-brand-lynx px-5 py-20 sm:px-8 sm:py-24 lg:px-10 lg:py-28">
            <div className="mx-auto max-w-7xl">
                <header className="mb-14 max-w-3xl sm:mb-20">
                    <h2 className="text-3xl font-bold leading-tight tracking-[-0.025em] text-brand-gunship sm:text-4xl lg:text-5xl">
                        Nossa <span className="border-b-4 border-brand-ember">Frota</span>
                    </h2>
                    <p className="mt-5 text-base leading-7 text-brand-gunship/80 sm:text-lg">
                        Veículos <span className="font-bold text-brand-gunship">modernos e equipados</span> para
                        garantir o máximo conforto em suas viagens
                    </p>
                </header>

                <div className="space-y-16 sm:space-y-20">
                    <FleetGroup
                        title="Ônibus"
                        vehicles={buses}
                        imageSrc="/assets/brand/schumacher-bus-cutout.webp"
                        imageAlt="Ônibus da frota Schumacher Tur"
                    />
                    <FleetGroup
                        title="Micro-ônibus e Vans"
                        vehicles={micros}
                        imageSrc="/assets/brand/schumacher-van-cutout.webp"
                        imageAlt="Van da frota Schumacher Tur"
                    />
                </div>
            </div>
        </section>
    )
}
