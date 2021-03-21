import React, {FC} from "react";
import {Card} from "@components/Card/Card";
import {Statistic} from "@components/Statistic/Statistic";
import {SiteStats} from "@app/queries/FetchSiteStats";

interface AppStatsProps {
	stats: SiteStats
}

export const AppStats: FC<AppStatsProps> = (props) => {
	const { playerCount, activeGames } = props.stats

	return (
		<Card noPad>
			<div className="grid grid-cols-2 md:grid-cols-1 xl:grid-cols-2">
				<Statistic
					orientation="center"
					title="Players Online"
					value={playerCount}
				/>
				<Statistic
					orientation="center"
					title="Active Games"
					value={activeGames}
				/>
			</div>
		</Card>
	)
}