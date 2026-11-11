import { createFileRoute } from "@tanstack/react-router"

import { SponsorBanner } from "@/components/sponsor-banner"
import { sponsors } from "@/sponsors"

const title = "Sponsors · MyGo"
const description = "Sponsor MyGo for $200 per month: your banner in the README and on the website."
const email = "hi@egoist.dev"

const benefits = [
  {
    title: "Banner in the README",
    text: "Your banner in the Sponsored by section at the top of the README.",
  },
  {
    title: "Banner on the website",
    text: "Your banner in the Sponsors section of the home page, and on this page.",
  },
]

export const Route = createFileRoute("/sponsors")({
  head: () => ({
    meta: [
      { title },
      { name: "description", content: description },
      { property: "og:title", content: title },
      { property: "og:description", content: description },
    ],
  }),
  component: Sponsors,
})

function Sponsors() {
  return (
    <main className="flex-1">
      <section className="border-b px-4 pt-16 pb-12 sm:px-10 md:pt-20">
        <p className="label">Sponsors</p>
        <h1 className="mt-5 text-3xl leading-[1.1] font-semibold tracking-[-0.035em] sm:text-4xl">Sponsor MyGo.</h1>
        <p className="mt-5 max-w-2xl text-base leading-7 text-muted-foreground">
          MyGo is free and open source. Sponsors support its development, and their banner is shown in the README and on this website.
        </p>
        <p className="mt-10 text-3xl font-semibold tracking-tight tabular-nums sm:text-4xl">
          $200
          <span className="ml-1 text-base font-normal text-muted-foreground">per month</span>
        </p>
      </section>

      <section className="border-b px-4 py-12 sm:px-10">
        <p className="label">Benefits</p>
        <ul className="mt-6 grid gap-8 sm:grid-cols-2">
          {benefits.map((benefit) => (
            <li key={benefit.title}>
              <h2 className="font-semibold tracking-tight">{benefit.title}</h2>
              <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">{benefit.text}</p>
            </li>
          ))}
        </ul>
      </section>

      <section className="border-b px-4 py-12 sm:px-10">
        <p className="label">Current sponsors</p>
        <ul className="mt-6 grid gap-6">
          {sponsors.map((sponsor) => (
            <li key={sponsor.url}>
              <SponsorBanner sponsor={sponsor} />
            </li>
          ))}
        </ul>
      </section>

      <section className="px-4 py-12 sm:px-10">
        <h2 className="text-xl font-semibold tracking-tight">Become a sponsor</h2>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
          To sponsor MyGo at $200 per month, email{" "}
          <a href={`mailto:${email}`} className="font-mono text-[13px] text-foreground underline underline-offset-4">
            {email}
          </a>
          .
        </p>
      </section>
    </main>
  )
}
