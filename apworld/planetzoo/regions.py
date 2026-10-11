from __future__ import annotations

from typing import TYPE_CHECKING

import typing

from .names import RegionNames, EntranceNames
from .locations import LocationFormation, research_locations_names, mechanic_locations_names, conservation_locations_names, milestones_locations_names, firsts_locations_names
from BaseClasses import Entrance, Region

if TYPE_CHECKING:
    from .world import PlanetZooWorld


class RegionInfo(typing.NamedTuple):
    name: str
    locations: typing.List[LocationFormation]


all_regions = [
    RegionInfo(RegionNames.menu, []),
    RegionInfo(RegionNames.research, research_locations_names),
    RegionInfo(RegionNames.mechanic, mechanic_locations_names),
    RegionInfo(RegionNames.conservation, conservation_locations_names),
    RegionInfo(RegionNames.milestones, milestones_locations_names),
    RegionInfo(RegionNames.firsts, firsts_locations_names)
]
regions_by_name: typing.Dict[str, RegionInfo] = {
    region.name: region for region in all_regions}


def create_and_connect_regions(world: PlanetZooWorld) -> None:
    create_all_regions(world)
    connect_regions(world)


def create_all_regions(world: PlanetZooWorld) -> None:
    menu = Region(RegionNames.menu, world.player, world.multiworld)
    research_locations = Region(
        RegionNames.research, world.player, world.multiworld)
    mechanic_locations = Region(
        RegionNames.mechanic, world.player, world.multiworld)
    conservation_locations = Region(
        RegionNames.conservation, world.player, world.multiworld)
    milestones_locations = Region(
        RegionNames.milestones, world.player, world.multiworld)
    firsts_locations = Region(
        RegionNames.firsts, world.player, world.multiworld)
    # goal_locations = Region("Goal Locations", world.player, world.multiworld)

    regions = [menu, research_locations, mechanic_locations, conservation_locations,
               milestones_locations, firsts_locations]  # goal_locations]

    # goal_locations.add_locations({"Goal Check":None}, PlanetZooLocation)
    # world.multiworld.get_location("Goal Check", world.player).place_locked_item(PlanetZooItem("Goal Achievable", ItemClassification.progression, None, world.player))
    # Option for if it only exists if player enables certain options
    # if world.options.placeholder_option:
    #     seperate_location = Region("Seperate Location", world.player, world.multiworld)
    #     regions.append(seperate_location)

    world.multiworld.regions += regions


def connect_regions(world: PlanetZooWorld) -> None:
    menu = world.multiworld.get_region(RegionNames.menu, world.player)
    research = world.multiworld.get_region(RegionNames.research, world.player)
    mechanic = world.multiworld.get_region(RegionNames.mechanic, world.player)
    conservation = world.multiworld.get_region(
        RegionNames.conservation, world.player)
    milestones = world.multiworld.get_region(
        RegionNames.milestones, world.player)
    firsts = world.multiworld.get_region(RegionNames.firsts, world.player)
    # goal = world.multiworld.get_region("Goal Locations", world.player)

    menu_to_research = Entrance(
        world.player, EntranceNames.menu_to_research, parent=menu)
    menu.exits.append(menu_to_research)
    menu_to_research.connect(research)

    menu_to_mechanic = Entrance(
        world.player, EntranceNames.menu_to_mechanic, parent=menu)
    menu.exits.append(menu_to_mechanic)
    menu_to_mechanic.connect(mechanic)

    menu_to_conservation = Entrance(
        world.player, EntranceNames.menu_to_conservation, parent=menu)
    menu.exits.append(menu_to_conservation)
    menu_to_conservation.connect(conservation)

    menu_to_milestones = Entrance(
        world.player, EntranceNames.menu_to_milestones, parent=menu)
    menu.exits.append(menu_to_milestones)
    menu_to_milestones.connect(milestones)

    menu_to_firsts = Entrance(
        world.player, EntranceNames.menu_to_firsts, parent=menu)
    menu.exits.append(menu_to_firsts)
    menu_to_firsts.connect(firsts)
