using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WeatherServiceTests
{
    [Fact]
    public void SourceGeneratedParsers_KeepCaseInsensitiveAndUnknownFieldBehavior()
    {
        WeatherGeocodingResult geocoding = Assert.IsType<WeatherGeocodingResult>(
            WeatherService.DeserializeGeocodingResponse(
                """
                {
                  "RESULTS": [
                    {
                      "NAME": "Paris",
                      "LATITUDE": 48.85,
                      "LONGITUDE": 2.35,
                      "COUNTRY": "France",
                      "futureCityField": true
                    }
                  ],
                  "futureRootField": "ignored"
                }
                """));
        WeatherGeocodingItem city = Assert.Single(geocoding.Results!);
        Assert.Equal("Paris", city.Name);
        Assert.Equal(48.85, city.Latitude);

        WeatherData openMeteo = Assert.IsType<WeatherData>(
            WeatherService.DeserializeOpenMeteoResponse(
                """
                {
                  "LATITUDE": 1.5,
                  "LONGITUDE": 2.5,
                  "CURRENT": {
                    "TEMPERATURE_2M": 23,
                    "futureCurrentField": 1
                  }
                }
                """));
        Assert.Equal(1.5, openMeteo.Latitude);
        Assert.Equal(23, Assert.IsType<WeatherCurrent>(openMeteo.Current).Temperature);

        MsnWeatherResponse msn = Assert.IsType<MsnWeatherResponse>(
            WeatherService.DeserializeMsnResponse(
                """
                {
                  "VALUE": [
                    {
                      "RESPONSES": [
                        {
                          "WEATHER": [
                            {
                              "CURRENT": {
                                "TEMP": 27,
                                "CAP": "Clear",
                                "futureMsnField": false
                              }
                            }
                          ]
                        }
                      ]
                    }
                  ]
                }
                """));
        MsnWeatherCurrent current = Assert.IsType<MsnWeatherCurrent>(
            Assert.Single(
                Assert.Single(
                    Assert.Single(msn.Value!).Responses!).Weather!).Current);
        Assert.Equal(27, current.Temp);
        Assert.Equal("Clear", current.Cap);
    }

    [Fact]
    public void ConvertMsnToWeatherData_UsesFollowingDayWhenCurrentDayHasNoHourlyRows()
    {
        var response = new MsnWeatherBody
        {
            Forecast = new MsnWeatherForecast
            {
                Days =
                [
                    new MsnForecastDay { Hourly = [] },
                    new MsnForecastDay
                    {
                        Hourly =
                        [
                            new MsnForecastHour
                            {
                                Valid = "2026-07-29T00:00:00+08:00",
                                Temp = 27,
                                Precip = 20,
                                Cap = "Partly cloudy",
                                Icon = 29
                            },
                            new MsnForecastHour
                            {
                                Valid = "2026-07-29T01:00:00+08:00",
                                Temp = 26,
                                Precip = 30,
                                Cap = "Cloudy",
                                Icon = 26
                            }
                        ]
                    }
                ]
            }
        };

        WeatherData data = WeatherService.ConvertMsnToWeatherData(response, 30.5, 114.3);

        Assert.NotNull(data.Hourly);
        Assert.Equal(2, data.Hourly.Time.Count);
        Assert.Equal([27d, 26d], data.Hourly.Temperature);
        Assert.Equal([20d, 30d], data.Hourly.PrecipitationProbability);
    }
}
