import React, {FC} from "react";
import {ContentContainer} from "@app/containers/ContentContainer";
import {Game} from "@app/components/Game/Game";

export const DemoView: FC = () => {
	return (
		<ContentContainer>
			<Game />
		</ContentContainer>
	)
}