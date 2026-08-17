import { buses, micros } from '../data/fleet'

function FleetGroup({ title, vehicles }) {
    return (
        <div>
            <div className="mb-6 flex items-center gap-4">
                <div className="h-px flex-1 bg-gradient-to-r from-transparent via-gold-200 to-transparent" />
                <h3 className="text-center text-2xl font-bold text-dark-800">{title}</h3>
                <div className="h-px flex-1 bg-gradient-to-r from-transparent via-gold-200 to-transparent" />
            </div>

            <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                {vehicles.map((vehicle) => (
                    <li
                        key={vehicle.id}
                        className="flex min-h-20 items-center rounded-2xl border border-light-200 bg-white px-6 py-5 shadow-sm transition-colors hover:border-gold-300"
                    >
                        <span className="mr-4 h-2.5 w-2.5 shrink-0 rounded-full bg-gold-400" />
                        <span className="font-heading text-lg font-bold text-dark-900">
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
        <section id="frota" className="section-padding relative overflow-hidden bg-light-50">
            <div className="absolute right-0 top-0 h-1/3 w-1/3 -translate-y-1/2 translate-x-1/2 rounded-full bg-gold-100/30 blur-3xl" />
            <div className="absolute bottom-0 left-0 h-1/4 w-1/4 -translate-x-1/3 translate-y-1/2 rounded-full bg-gold-200/20 blur-3xl" />

            <div className="container-max relative z-10">
                <div className="mb-12 text-center sm:mb-16">
                    <h2 className="section-title">
                        Nossa <span className="text-gradient-gold">Frota</span>
                    </h2>
                    <p className="section-subtitle">
                        Veículos <span className="font-semibold text-gold-500">modernos e equipados</span> para
                        garantir o máximo conforto em suas viagens
                    </p>
                </div>

                <div className="space-y-12">
                    <FleetGroup title="Ônibus" vehicles={buses} />
                    <FleetGroup title="Micro-ônibus e Vans" vehicles={micros} />
                </div>
            </div>
        </section>
    )
}
