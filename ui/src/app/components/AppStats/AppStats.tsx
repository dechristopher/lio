import React, {FC} from "react";
import {Card} from "@components/Card/Card";
import {Statistic} from "@components/Statistic/Statistic";

export const AppStats: FC = () => {
	return (
		<Card noPad>
			<div className="grid grid-cols-2 md:grid-cols-1 xl:grid-cols-2">
				<Statistic
					orientation="center"
					title="Players Online"
					value={Math.round(Math.random() * 3000) + 1000}
				/>
				<Statistic
					orientation="center"
					title="Active Games"
					value={Math.round(Math.random() * 1500) + 1000}
				/>
			</div>
		</Card>
	)
}