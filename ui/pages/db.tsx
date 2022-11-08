import ContentWrapper from "@/components/ContentWrapper/ContentWrapper";
import React from "react";

const DB = () => {
	return (
		<ContentWrapper>
			<div className="text-center w-80">
				<div className="text-xl font-bold">Game Database</div>
				<div className="prose mt-3">
					Eventually, all games played will be made available here for
					download. They will be combined into large monthly dump
					files of all raw PGNs in chronological order.
				</div>
				<div className="prose mt-3">
					Until that day we will continue to build the site.
				</div>
			</div>
		</ContentWrapper>
	);
};

export default DB;
