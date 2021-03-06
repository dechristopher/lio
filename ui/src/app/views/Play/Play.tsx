import React, {FC, useEffect, useState} from "react";
import {TopPlayersList} from "@app/components/TopPlayersList/TopPlayersList";
import {AppStats} from "@app/components/AppStats/AppStats";
import {Card} from "@components/Card/Card";
import {Footer} from "@app/components/Footer/Footer";
import {CustomGames} from "@app/components/CustomGames/CustomGames";
import {RatedGameTab} from "@app/components/GameTabs/RatedGameTab";
import {CreateGameTab} from "@app/components/GameTabs/CreateGameTab";
import Tabs from "@components/Tabs/Tabs";
import {ContentContainer} from "@app/containers/ContentContainer";
import {FetchSiteStats, SiteStats} from "@app/queries/FetchSiteStats";

const initSiteStats: SiteStats = {
	playerCount: 0,
	activeGames: 0
}

export const PlayView: FC = () => {
	const [siteStats, setSiteStats] = useState<SiteStats>(initSiteStats)

	/**
	 * Fetch site stats when this component mounts.
	 */
	useEffect(() => {
		const fetchAndSetSiteStats = () => {
			FetchSiteStats()
				.then(stats => setSiteStats(stats))
		}

		// initial call to get site stats
		fetchAndSetSiteStats();

		// run fetchAndSetSiteStats every 5 minutes
		const intervalId = setInterval(() => {
			fetchAndSetSiteStats()
		}, 300000)

		// clear the set interval when the component unmounts
		return () => {
			clearInterval(intervalId)
		}
	}, [])

	return (
		<ContentContainer>
			<div className="md:flex md:items-center md:justify-between pt-8 pb-36 bg-green-500 shadow-lg">
				<div className="px-6 flex-1 min-w-0">
					<h2 className="text-4xl font-bold leading-12 text-white">
						Play
					</h2>
				</div>
			</div>
			<div className="flex-1 relative z-0 flex -mt-36">
				<main className="flex-1 relative z-0 focus:outline-none space-y-8" tabIndex={0}>
					<div className="absolute inset-x-0 top-0 py-6 px-2 sm:px-4 lg:px-4">
					{/* <!-- Start main area--> */}

						{/* Online player stats */}
						<div className="block md:hidden">
							<AppStats stats={siteStats} />
						</div>

						{/* Game variants */}
						<div className="mt-6 md:mt-0">
							<Card noPad>
								<Tabs>
									<Tabs.Tab
										title="Play Rated Game"
										content={<RatedGameTab />}
									/>
									<Tabs.Tab
										title="Create Game"
										content={<CreateGameTab />}
									/>
								</Tabs>
							</Card>
						</div>

						{/* Custom games */}
						<div className="mt-6">
							<CustomGames />
						</div>

						{/* Top players */}
						<div className="block md:hidden mt-6 md:mt-0">
							<TopPlayersList />
						</div>

						{/* Footer */}
						<div className="block md:hidden mt-6 pb-8 md:mt-0 md:pb-0">
							<Footer />
						</div>
					{/* <!-- End main area --> */}
					</div>
				</main>
				<aside className="hidden relative md:flex md:flex-col flex-shrink-0 w-4/12">
					{/* <!-- Start secondary column (hidden on smaller screens) --> */}
					<div className="absolute inset-0 py-6 px-2 sm:px-4 lg:px-4 space-y-8">
						<TopPlayersList />

						<AppStats stats={siteStats} />

						<Footer />
					</div>
					{/* <!-- End secondary column --> */}
				</aside>
			</div>
		</ContentContainer>
	)
}