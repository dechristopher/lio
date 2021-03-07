import React, {FC} from "react";
import {TopPlayersList} from "@app/components/TopPlayersList/TopPlayersList";
import {AppStats} from "@app/components/AppStats/AppStats";
import Tabs from "@app/components/Tabs/Tabs";
import {Card} from "@components/Card/Card";
import {Footer} from "@app/components/Footer/Footer";
import {CustomGames} from "@app/components/CustomGames/CustomGames";

export const PlayView: FC = () => {
	return (
		<div className="mt-16 w-screen flex flex-col items-stretch overflow-x-hidden overflow-y-auto" style={{height: "calc(100vh - 4rem)"}}>
			<div className="md:flex md:items-center md:justify-between pt-8 pb-36 bg-green-500 shadow-lg">
				<div className="px-6 flex-1 min-w-0">
					<h2 className="text-4xl font-bold leading-12 text-white">
						Play
					</h2>
				</div>
			</div>
			<div className="flex-1 relative z-0 flex -mt-36">
				<main className="flex-1 relative z-0 focus:outline-none space-y-8" tabIndex={0}>
					<div className="absolute inset-0 py-6 px-2 sm:px-4 lg:px-4">
					{/* <!-- Start main area--> */}

						{/* Online player stats */}
						<div className="block md:hidden">
							<AppStats />
						</div>

						{/* Game variants */}
						<div className="mt-6 md:mt-0">
							<Card noPad>
									<Tabs>
										<Tabs.Tab title="Play Rated Game" content={<div className="h-96">Rated Game Presets</div>} />
										<Tabs.Tab title="Create Game" content={<div className="h-96">Custom Game Options</div>} />
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

						<AppStats />

						<Footer />
					</div>
					{/* <!-- End secondary column --> */}
				</aside>
			</div>
		</div>
	)
}