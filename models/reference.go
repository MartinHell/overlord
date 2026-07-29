package models

import "strings"

// Reference data mapping DCS's internal type identifiers to something readable.
//
// DCS knows the display name -- unit:getDesc() carries it -- but DCS-gRPC only
// exposes the attribute list, not the name, so there is nothing to read it from
// over the wire. Until that changes upstream this table is curated, with
// prettify() as the fallback for anything not listed.
//
// Deliberately limited to identity: designation, common name, role, origin and
// manufacturer. No performance figures. Speeds, ceilings and ranges would have
// to be sourced rather than recalled, and a wrong number is worse than an absent
// one to the audience this is for. Add them from a source you trust if you want
// them.
type Profile struct {
	// Name is what a person calls it.
	Name string
	// Nickname is the popular or NATO reporting name, where one exists.
	Nickname string
	Role     string
	Origin   string
	Maker    string
	// Blurb is a sentence of context, not a spec sheet.
	Blurb string
}

// aircraft, helicopters, ground vehicles and air defence, keyed by DCS type.
var unitProfiles = map[string]Profile{
	"F-14B": {"F-14B Tomcat", "Tomcat", "Carrier-based interceptor and multirole fighter", "United States", "Grumman",
		"Two-seat swing-wing carrier fighter, built around the AWG-9 radar and the AIM-54 Phoenix for long-range fleet defence."},
	"F-15C": {"F-15C Eagle", "Eagle", "Air superiority fighter", "United States", "McDonnell Douglas",
		"Single-seat air superiority fighter. In DCS it is a flaming cliffs aircraft, so it is flown without a clickable cockpit."},
	"F-15E": {"F-15E Strike Eagle", "Mudhen", "Strike fighter", "United States", "McDonnell Douglas",
		"Two-seat derivative of the Eagle that keeps the air-to-air capability while adding a dedicated strike role."},
	"F-16C_50": {"F-16C Block 50", "Viper", "Multirole fighter", "United States", "General Dynamics",
		"Single-engine multirole fighter. The Block 50 is the DCS variant, with the HARM targeting system for defence suppression."},
	"FA-18C_hornet": {"F/A-18C Hornet", "Hornet", "Carrier-based multirole fighter", "United States", "McDonnell Douglas",
		"Carrier-capable multirole fighter, equally at home in the air-to-air and air-to-ground roles."},
	"A-10A":   {"A-10A Thunderbolt II", "Warthog", "Close air support", "United States", "Fairchild Republic", "Built around its gun for close air support."},
	"A-10C":   {"A-10C Thunderbolt II", "Warthog", "Close air support", "United States", "Fairchild Republic", "Upgraded Thunderbolt with precision munitions and a modern cockpit."},
	"AV8BNA":  {"AV-8B Harrier II N/A", "Harrier", "V/STOL attack", "United States", "McDonnell Douglas", "Vertical and short takeoff attack aircraft."},
	"F-4E":    {"F-4E Phantom II", "Phantom", "Multirole fighter", "United States", "McDonnell Douglas", "Two-seat third-generation fighter-bomber."},
	"M-2000C": {"Mirage 2000C", "Mirage", "Multirole fighter", "France", "Dassault", "Delta-wing multirole fighter."},
	"AJS37":   {"AJS 37 Viggen", "Viggen", "Strike fighter", "Sweden", "Saab", "Cold war strike aircraft built for dispersed road basing."},

	"Su-27":     {"Su-27 Flanker-B", "Flanker", "Air superiority fighter", "Soviet Union", "Sukhoi", "Large twin-engine air superiority fighter."},
	"Su-33":     {"Su-33 Flanker-D", "Flanker", "Carrier-based fighter", "Soviet Union", "Sukhoi", "Navalised Flanker for the Kuznetsov."},
	"Su-25":     {"Su-25 Frogfoot", "Frogfoot", "Close air support", "Soviet Union", "Sukhoi", "Armoured close air support aircraft."},
	"Su-25T":    {"Su-25T Frogfoot", "Frogfoot", "Anti-armour attack", "Soviet Union", "Sukhoi", "Frogfoot variant with precision anti-armour capability."},
	"Su-24M":    {"Su-24M Fencer-D", "Fencer", "All-weather strike bomber", "Soviet Union", "Sukhoi", "Swing-wing two-seat strike bomber."},
	"Su-34":     {"Su-34 Fullback", "Fullback", "Strike fighter", "Russia", "Sukhoi", "Side-by-side two-seat strike derivative of the Flanker."},
	"MiG-29S":   {"MiG-29S Fulcrum-C", "Fulcrum", "Multirole fighter", "Soviet Union", "Mikoyan", "Improved Fulcrum with expanded air-to-air capability."},
	"MiG-31":    {"MiG-31 Foxhound", "Foxhound", "Long-range interceptor", "Soviet Union", "Mikoyan", "High-speed, high-altitude interceptor built to engage at long range."},
	"MiG-27K":   {"MiG-27K", "Flogger", "Ground attack", "Soviet Union", "Mikoyan", "Swing-wing ground attack aircraft."},
	"MiG-21Bis": {"MiG-21bis Fishbed", "Fishbed", "Interceptor", "Soviet Union", "Mikoyan", "Lightweight delta-wing interceptor."},
	"Tu-22M3":   {"Tu-22M3 Backfire-C", "Backfire", "Strategic bomber", "Soviet Union", "Tupolev", "Swing-wing supersonic bomber and maritime strike platform."},
	"Tu-95MS":   {"Tu-95MS Bear-H", "Bear", "Strategic bomber", "Soviet Union", "Tupolev", "Turboprop strategic bomber and cruise missile carrier."},

	"A-50":    {"A-50 Mainstay", "Mainstay", "Airborne early warning", "Soviet Union", "Beriev", "AEW&C aircraft, the eastern counterpart to the E-3."},
	"E-3A":    {"E-3A Sentry", "Sentry", "Airborne early warning", "United States", "Boeing", "AWACS aircraft directing the air picture."},
	"IL-78M":  {"Il-78M Midas", "Midas", "Aerial refuelling tanker", "Soviet Union", "Ilyushin", "Probe-and-drogue tanker."},
	"IL-76MD": {"Il-76MD Candid", "Candid", "Strategic transport", "Soviet Union", "Ilyushin", "Four-engine military freighter."},
	"An-26B":  {"An-26B Curl", "Curl", "Tactical transport", "Soviet Union", "Antonov", "Twin-turboprop tactical transport."},
	"C-130":   {"C-130 Hercules", "Herc", "Tactical transport", "United States", "Lockheed", "Four-engine turboprop transport."},
	"C-17A":   {"C-17A Globemaster III", "Globemaster", "Strategic transport", "United States", "Boeing", "Heavy strategic airlifter."},
	"KC-135":  {"KC-135 Stratotanker", "Stratotanker", "Aerial refuelling tanker", "United States", "Boeing", "Boom-equipped tanker."},

	"AH-64A": {"AH-64A Apache", "Apache", "Attack helicopter", "United States", "Hughes", "Attack helicopter carrying the Hellfire."},
	"AH-64D": {"AH-64D Apache Longbow", "Apache", "Attack helicopter", "United States", "Boeing", "Apache with the mast-mounted Longbow radar."},
	"UH-60A": {"UH-60A Black Hawk", "Black Hawk", "Utility helicopter", "United States", "Sikorsky", "Utility transport helicopter."},
	"CH-47D": {"CH-47D Chinook", "Chinook", "Heavy transport helicopter", "United States", "Boeing", "Tandem-rotor heavy lift helicopter."},
	"Mi-8MT": {"Mi-8MTV-2 Hip", "Hip", "Transport helicopter", "Soviet Union", "Mil", "Widely used medium transport helicopter."},
	"Mi-24P": {"Mi-24P Hind-F", "Hind", "Attack and transport helicopter", "Soviet Union", "Mil", "Gunship that also carries troops, with a fixed side-mounted cannon."},
	"Mi-26":  {"Mi-26 Halo", "Halo", "Heavy transport helicopter", "Soviet Union", "Mil", "The heaviest production helicopter."},
	"Ka-50":  {"Ka-50 Black Shark", "Black Shark", "Attack helicopter", "Russia", "Kamov", "Single-seat coaxial-rotor attack helicopter."},
	"SA342M": {"SA 342M Gazelle", "Gazelle", "Light attack helicopter", "France", "Aérospatiale", "Light scout and anti-tank helicopter."},

	"M-1 Abrams":    {"M1A2 Abrams", "Abrams", "Main battle tank", "United States", "General Dynamics", "Gas turbine main battle tank."},
	"M-2 Bradley":   {"M2A2 Bradley", "Bradley", "Infantry fighting vehicle", "United States", "FMC", "Tracked IFV with a chain gun and TOW launcher."},
	"M1097 Avenger": {"M1097 Avenger", "Avenger", "Short-range air defence", "United States", "Boeing", "Humvee-mounted Stinger launcher."},
	"T-72B":         {"T-72B", "", "Main battle tank", "Soviet Union", "Uralvagonzavod", "Widely exported main battle tank."},
	"T-80UD":        {"T-80UD", "", "Main battle tank", "Soviet Union", "Malyshev", "Gas turbine main battle tank."},
	"BMP-2":         {"BMP-2", "", "Infantry fighting vehicle", "Soviet Union", "Kurganmashzavod", "Tracked IFV with a 30mm cannon and ATGM."},
	"BTR-80":        {"BTR-80", "", "Armoured personnel carrier", "Soviet Union", "GAZ", "Eight-wheeled amphibious APC."},
	"Ural-375":      {"Ural-375D", "", "Utility truck", "Soviet Union", "UralAZ", "General purpose military truck."},

	"Hawk tr":              {"Hawk tracking radar", "", "Air defence radar", "United States", "Raytheon", "Tracking radar for the MIM-23 Hawk system."},
	"Roland ADS":           {"Roland ADS", "Roland", "Short-range air defence", "France and West Germany", "Euromissile", "Mobile short-range SAM launcher."},
	"Strela-10M3":          {"9K35 Strela-10", "Gopher", "Short-range air defence", "Soviet Union", "KBP", "Tracked short-range infrared SAM."},
	"Tor 9A331":            {"9K330 Tor", "Gauntlet", "Short-range air defence", "Soviet Union", "Almaz-Antey", "Tracked launcher with its own radar, effective against precision munitions."},
	"SA-11 Buk LN 9A310M1": {"9K37 Buk launcher", "Gadfly", "Medium-range air defence", "Soviet Union", "Almaz-Antey", "Self-propelled medium-range SAM launcher with an onboard radar."},
}

// weaponProfiles keys on DCS weapon type. Gun ammunition is grouped by the gun
// that fires it, since the shell name is what DCS reports.
var weaponProfiles = map[string]Profile{
	"AIM_54C_Mk47": {"AIM-54C Phoenix", "Phoenix", "Long-range air-to-air missile", "United States", "Hughes",
		"Long-range active radar missile carried only by the F-14, and the reason the Tomcat exists in the fleet defence role."},
	"AIM_120":  {"AIM-120A AMRAAM", "Slammer", "Medium-range air-to-air missile", "United States", "Hughes", "Active radar homing missile, fire and forget after launch."},
	"AIM_120C": {"AIM-120C AMRAAM", "Slammer", "Medium-range air-to-air missile", "United States", "Raytheon", "Later AMRAAM with clipped fins for internal carriage."},
	"AIM_9":    {"AIM-9 Sidewinder", "Sidewinder", "Short-range air-to-air missile", "United States", "Raytheon", "Infrared homing dogfight missile."},
	"AIM_9X":   {"AIM-9X Sidewinder", "Sidewinder", "Short-range air-to-air missile", "United States", "Raytheon", "High off-boresight imaging infrared Sidewinder."},
	"AIM_7":    {"AIM-7 Sparrow", "Sparrow", "Medium-range air-to-air missile", "United States", "Raytheon", "Semi-active radar missile requiring the launcher to keep a lock."},

	"P_27P":  {"R-27R Alamo-A", "Alamo", "Medium-range air-to-air missile", "Soviet Union", "Vympel", "Semi-active radar guided missile."},
	"P_27PE": {"R-27ER Alamo-C", "Alamo", "Long-range air-to-air missile", "Soviet Union", "Vympel", "Extended-range semi-active radar missile with a larger motor."},
	"P_27TE": {"R-27ET Alamo-D", "Alamo", "Long-range air-to-air missile", "Soviet Union", "Vympel", "Infrared guided extended-range Alamo, with no launch warning for the target."},
	"P_33E":  {"R-33 Amos", "Amos", "Long-range air-to-air missile", "Soviet Union", "Vympel", "Long-range missile carried by the MiG-31."},
	"P_40T":  {"R-40T Acrid", "Acrid", "Long-range air-to-air missile", "Soviet Union", "Bisnovat", "Large infrared guided interceptor missile."},
	"P_60":   {"R-60 Aphid", "Aphid", "Short-range air-to-air missile", "Soviet Union", "Vympel", "Lightweight infrared dogfight missile."},
	"P_73":   {"R-73 Archer", "Archer", "Short-range air-to-air missile", "Soviet Union", "Vympel", "High off-boresight infrared missile."},

	"AGM_88":   {"AGM-88 HARM", "HARM", "Anti-radiation missile", "United States", "Texas Instruments", "Homes on radar emissions to suppress air defences."},
	"AGM_154":  {"AGM-154 JSOW", "JSOW", "Glide bomb", "United States", "Raytheon", "Unpowered glide weapon that dispenses submunitions, which is why it registers many hits per launch."},
	"AGM_65D":  {"AGM-65D Maverick", "Maverick", "Air-to-ground missile", "United States", "Hughes", "Imaging infrared Maverick for armour."},
	"AGM_65H":  {"AGM-65H Maverick", "Maverick", "Air-to-ground missile", "United States", "Raytheon", "CCD television guided Maverick."},
	"AGM_114K": {"AGM-114K Hellfire II", "Hellfire", "Anti-armour missile", "United States", "Lockheed Martin", "Laser guided helicopter-launched anti-armour missile."},
	"X_25MP":   {"Kh-25MP", "Karen", "Anti-radiation missile", "Soviet Union", "Zvezda", "Short-range anti-radiation missile."},
	"X_31P":    {"Kh-31P", "Krypton", "Anti-radiation missile", "Soviet Union", "Zvezda", "Supersonic anti-radiation missile."},
	"X_58":     {"Kh-58", "Kilter", "Anti-radiation missile", "Soviet Union", "Raduga", "Long-range anti-radiation missile."},

	"GBU_12":  {"GBU-12 Paveway II", "", "Laser guided bomb", "United States", "Raytheon", "500lb laser guided bomb."},
	"GBU_31":  {"GBU-31 JDAM", "JDAM", "GPS guided bomb", "United States", "Boeing", "2000lb satellite guided bomb."},
	"GBU_38":  {"GBU-38 JDAM", "JDAM", "GPS guided bomb", "United States", "Boeing", "500lb satellite guided bomb."},
	"KAB_500": {"KAB-500Kr", "", "Guided bomb", "Soviet Union", "Region", "Television guided 500kg bomb."},
	"RBK_250": {"RBK-250", "", "Cluster bomb", "Soviet Union", "Basalt", "Cluster dispenser, which registers several hits per release."},

	"TOW2":     {"BGM-71 TOW 2", "TOW", "Anti-tank guided missile", "United States", "Hughes", "Wire guided anti-tank missile."},
	"KONKURS":  {"9M113 Konkurs", "Spandrel", "Anti-tank guided missile", "Soviet Union", "KBP", "Wire guided anti-tank missile."},
	"AT_6":     {"9K114 Shturm", "Spiral", "Anti-tank guided missile", "Soviet Union", "KBM", "Radio guided helicopter-launched anti-tank missile."},
	"FIM_92C":  {"FIM-92C Stinger", "Stinger", "Man-portable air defence missile", "United States", "Raytheon", "Shoulder or vehicle launched infrared SAM."},
	"ROLAND_R": {"Roland missile", "Roland", "Short-range surface-to-air missile", "France and West Germany", "Euromissile", "Command guided short-range SAM."},
	"SA9M330":  {"9M330 for Tor", "Gauntlet", "Short-range surface-to-air missile", "Soviet Union", "Almaz-Antey", "Command guided missile for the Tor system."},
	"SA9M333":  {"9M333 for Strela-10", "Gopher", "Short-range surface-to-air missile", "Soviet Union", "KBM", "Infrared guided missile for the Strela-10."},
	"SA9M38M1": {"9M38M1 for Buk", "Gadfly", "Medium-range surface-to-air missile", "Soviet Union", "Almaz-Antey", "Semi-active radar missile for the Buk system."},

	"weapons.shells.M61_20_HE":        {"M61 Vulcan 20mm HE", "Vulcan", "Aircraft cannon ammunition", "United States", "General Electric", "20mm rotary cannon round. DCS reports gun hits but no shot events, so guns have no hits-per-shot figure."},
	"weapons.shells.M56A3_HE_RED":     {"57mm HE", "", "Anti-aircraft gun ammunition", "Soviet Union", "", "Anti-aircraft artillery round."},
	"weapons.shells.M2_12_7":          {"M2 Browning 12.7mm", "Ma Deuce", "Heavy machine gun ammunition", "United States", "Browning", "Heavy machine gun round."},
	"weapons.shells.M2_12_7_T":        {"M2 Browning 12.7mm tracer", "Ma Deuce", "Heavy machine gun ammunition", "United States", "Browning", "Tracer round."},
	"weapons.shells.Utes_12_7x108":    {"NSV Utes 12.7mm", "", "Heavy machine gun ammunition", "Soviet Union", "", "Heavy machine gun round."},
	"weapons.shells.Utes_12_7x108_T":  {"NSV Utes 12.7mm tracer", "", "Heavy machine gun ammunition", "Soviet Union", "", "Tracer round."},
	"weapons.shells.2A42_30_HE":       {"2A42 30mm HE", "", "Autocannon ammunition", "Soviet Union", "KBP", "30mm autocannon round."},
	"weapons.shells.2A42_30_AP":       {"2A42 30mm AP", "", "Autocannon ammunition", "Soviet Union", "KBP", "Armour piercing autocannon round."},
	"weapons.shells.2A46M_125_AP":     {"2A46M 125mm APFSDS", "", "Tank gun ammunition", "Soviet Union", "", "Fin-stabilised armour piercing tank round."},
	"weapons.shells.2A46M_125_HE":     {"2A46M 125mm HE", "", "Tank gun ammunition", "Soviet Union", "", "High explosive tank round."},
	"weapons.shells.M256_120_AP":      {"M256 120mm APFSDS", "", "Tank gun ammunition", "United States", "", "Fin-stabilised armour piercing tank round."},
	"weapons.shells.M256_120_HE":      {"M256 120mm HE", "", "Tank gun ammunition", "United States", "", "High explosive tank round."},
	"weapons.shells.M242_25_AP_M791":  {"M242 Bushmaster 25mm AP", "Bushmaster", "Autocannon ammunition", "United States", "", "Armour piercing autocannon round."},
	"weapons.shells.M242_25_HE_M792":  {"M242 Bushmaster 25mm HE", "Bushmaster", "Autocannon ammunition", "United States", "", "High explosive autocannon round."},
	"weapons.shells.M53_APT_RED":      {"57mm APT", "", "Anti-aircraft gun ammunition", "Soviet Union", "", "Armour piercing tracer round."},
	"weapons.shells.7_62x54":          {"7.62x54mm", "", "Machine gun ammunition", "Soviet Union", "", "Rifle calibre machine gun round."},
	"weapons.shells.7_62x54_NOTRACER": {"7.62x54mm", "", "Machine gun ammunition", "Soviet Union", "", "Rifle calibre machine gun round."},
}

// UnitProfile returns reference data for a unit type. The second result reports
// whether the type is actually in the table, so callers can distinguish curated
// data from a prettified guess.
func UnitProfile(unitType string) (Profile, bool) {
	if p, ok := unitProfiles[unitType]; ok {
		return p, true
	}
	return Profile{Name: prettify(unitType)}, false
}

// WeaponProfile returns reference data for a weapon type.
func WeaponProfile(weaponType string) (Profile, bool) {
	if p, ok := weaponProfiles[weaponType]; ok {
		return p, true
	}
	return Profile{Name: prettify(weaponType)}, false
}

// prettify makes an unlisted DCS identifier readable without pretending to know
// what it is: strip the namespace, swap separators for spaces, tidy spacing.
// "weapons.shells.M61_20_HE" becomes "M61 20 HE".
func prettify(id string) string {
	name := id
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}

	name = strings.ReplaceAll(name, "_", " ")

	return strings.Join(strings.Fields(name), " ")
}
